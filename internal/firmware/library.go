package firmware

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Image is one firmware file in the library. It's stored as
// <family>fw-<version>.bin and served to phones as <family>fw.bin.
type Image struct {
	File    string    `json:"file"`
	Family  string    `json:"family"`
	Version Version   `json:"version"`
	Size    int64     `json:"size"`
	SHA256  string    `json:"sha256"`
	AddedAt time.Time `json:"addedAt"`
	Source  string    `json:"source"` // "file", "zip" or the download host
}

// ServedName is the file name a phone of this family asks for.
func (i Image) ServedName() string { return i.Family + "fw.bin" }

// Library is a folder of images plus an index with their versions.
type Library struct {
	dir string
	mu  sync.Mutex
	idx map[string]Image
}

// ErrNeedVersion is returned when a file's version can't be read from its
// name; the caller should ask the user and try again with AddFileAs.
var ErrNeedVersion = errors.New("version unknown")

func OpenLibrary(dir string) (*Library, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	l := &Library{dir: dir, idx: map[string]Image{}}
	if b, err := os.ReadFile(filepath.Join(dir, "index.json")); err == nil {
		json.Unmarshal(b, &l.idx)
	}
	l.reconcile()
	return l, nil
}

func (l *Library) Dir() string { return l.dir }

func (l *Library) saveLocked() error {
	b, _ := json.MarshalIndent(l.idx, "", "  ")
	return os.WriteFile(filepath.Join(l.dir, "index.json"), b, 0o600)
}

// reconcile drops index entries whose file is gone and indexes files that
// were copied into the folder by hand (when their name says the version).
func (l *Library) reconcile() {
	l.mu.Lock()
	defer l.mu.Unlock()
	changed := false
	for name := range l.idx {
		if _, err := os.Stat(filepath.Join(l.dir, name)); err != nil {
			delete(l.idx, name)
			changed = true
		}
	}
	ents, _ := os.ReadDir(l.dir)
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".bin") {
			continue
		}
		if _, ok := l.idx[e.Name()]; ok {
			continue
		}
		fam, ver := FamilyFromFile(e.Name()), Version{}
		if v, ok := ParseVersion(e.Name()); ok {
			ver = v
		}
		if fam == "" || ver.IsZero() {
			continue
		}
		if img, err := describe(filepath.Join(l.dir, e.Name()), fam, ver, "file"); err == nil {
			l.idx[e.Name()] = img
			changed = true
		}
	}
	if changed {
		l.saveLocked()
	}
}

// Rescan picks up files added to or removed from the folder by hand.
func (l *Library) Rescan() { l.reconcile() }

var safeFile = regexp.MustCompile(`^[A-Za-z0-9._+-]{1,200}$`)

func describe(p, family string, ver Version, source string) (Image, error) {
	st, err := os.Stat(p)
	if err != nil {
		return Image{}, err
	}
	f, err := os.Open(p)
	if err != nil {
		return Image{}, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return Image{}, err
	}
	return Image{File: filepath.Base(p), Family: family, Version: ver, Size: st.Size(), SHA256: hex.EncodeToString(h.Sum(nil)), AddedAt: time.Now(), Source: source}, nil
}

// List returns the indexed images, newest first within each family.
func (l *Library) List() []Image {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Image, 0, len(l.idx))
	for _, img := range l.idx {
		out = append(out, img)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Family != out[j].Family {
			return out[i].Family < out[j].Family
		}
		return out[i].Version.Compare(out[j].Version) > 0
	})
	return out
}

// Get returns the image by file name.
func (l *Library) Get(file string) (Image, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	img, ok := l.idx[file]
	return img, ok
}

// Path returns the full path of an image in the library, refusing anything
// that would escape the folder.
func (l *Library) Path(file string) (string, error) {
	if !safeFile.MatchString(file) || file == "index.json" {
		return "", errors.New("bad file name")
	}
	return filepath.Join(l.dir, file), nil
}

// Latest returns the newest image for a family, if any.
func (l *Library) Latest(family string) (Image, bool) {
	var best Image
	found := false
	for _, img := range l.List() {
		if img.Family == family && (!found || img.Version.Newer(best.Version)) {
			best, found = img, true
		}
	}
	return best, found
}

// AddFile copies a local image or a Grandstream release zip into the
// library. The version comes from the file name; if it isn't there,
// ErrNeedVersion is returned and the caller can use AddFileAs.
func (l *Library) AddFile(src string) (Image, error) {
	return l.AddFileAs(src, "", "")
}

// AddFileAs is AddFile with the family and version given by the caller
// (either may be empty to read it from the file name).
func (l *Library) AddFileAs(src, family, version string) (Image, error) {
	name := filepath.Base(src)
	lower := strings.ToLower(name)
	var ver Version
	if version != "" {
		v, ok := ParseVersion(version)
		if !ok {
			return Image{}, fmt.Errorf("%q isn't a version like 1.0.13.31", version)
		}
		ver = v
	} else if v, ok := ParseVersion(name); ok {
		ver = v
	}
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return l.addZip(src, family, ver)
	case strings.HasSuffix(lower, ".bin"):
		if family == "" {
			family = FamilyFromFile(name)
		}
		if family == "" {
			return Image{}, fmt.Errorf("can't tell which phones %s is for; Grandstream names images like grp2610fw.bin", name)
		}
		if ver.IsZero() {
			return Image{}, ErrNeedVersion
		}
		return l.copyIn(src, family, ver, "file")
	}
	return Image{}, errors.New("add a .bin image or the .zip Grandstream ships it in")
}

func (l *Library) addZip(src, family string, ver Version) (Image, error) {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return Image{}, err
	}
	defer zr.Close()
	var bin *zip.File
	for _, f := range zr.File {
		n := strings.ToLower(path.Base(f.Name))
		if strings.HasSuffix(n, "fw.bin") || (strings.HasSuffix(n, ".bin") && bin == nil) {
			bin = f
		}
	}
	if bin == nil {
		return Image{}, errors.New("the zip holds no .bin firmware image")
	}
	if family == "" {
		family = FamilyFromFile(path.Base(bin.Name))
	}
	if family == "" {
		return Image{}, fmt.Errorf("can't tell which phones %s is for", path.Base(bin.Name))
	}
	if ver.IsZero() {
		if v, ok := ParseVersion(path.Base(bin.Name)); ok {
			ver = v
		} else {
			return Image{}, ErrNeedVersion
		}
	}
	r, err := bin.Open()
	if err != nil {
		return Image{}, err
	}
	defer r.Close()
	dst := filepath.Join(l.dir, storedName(family, ver))
	tmp := dst + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return Image{}, err
	}
	if _, err := io.Copy(out, io.LimitReader(r, 512<<20)); err != nil {
		out.Close()
		os.Remove(tmp)
		return Image{}, err
	}
	out.Close()
	return l.finish(tmp, dst, family, ver, "zip")
}

func storedName(family string, ver Version) string {
	return fmt.Sprintf("%sfw-%s.bin", family, ver.Text)
}

func (l *Library) copyIn(src, family string, ver Version, source string) (Image, error) {
	dst := filepath.Join(l.dir, storedName(family, ver))
	tmp := dst + ".part"
	in, err := os.Open(src)
	if err != nil {
		return Image{}, err
	}
	defer in.Close()
	out, err := os.Create(tmp)
	if err != nil {
		return Image{}, err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return Image{}, err
	}
	out.Close()
	return l.finish(tmp, dst, family, ver, source)
}

func (l *Library) finish(tmp, dst, family string, ver Version, source string) (Image, error) {
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return Image{}, err
	}
	img, err := describe(dst, family, ver, source)
	if err != nil {
		os.Remove(dst)
		return Image{}, err
	}
	l.mu.Lock()
	l.idx[img.File] = img
	l.saveLocked()
	l.mu.Unlock()
	return img, nil
}

// Progress reports download progress; total is -1 when unknown.
type Progress func(done, total int64)

// Download fetches an image or release zip from an http(s) link, such as one
// from grandstream.com/support/firmware.
func (l *Library) Download(ctx context.Context, link, family, version string, progress Progress) (Image, error) {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return Image{}, errors.New("paste a full http(s) download link")
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	req.Header.Set("User-Agent", "PhoneDesk")
	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		return Image{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return Image{}, fmt.Errorf("download failed: %s", resp.Status)
	}
	if ct := resp.Header.Get("Content-Type"); strings.HasPrefix(ct, "text/html") {
		return Image{}, errors.New("the link returned a web page, not a file")
	}
	name := ""
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if m := regexp.MustCompile(`filename\*?=(?:UTF-8'')?"?([^";]+)"?`).FindStringSubmatch(cd); m != nil {
			name = path.Base(m[1])
		}
	}
	if name == "" {
		name = path.Base(resp.Request.URL.Path)
	}
	name = cleanName(name)
	lower := strings.ToLower(name)
	if !strings.HasSuffix(lower, ".bin") && !strings.HasSuffix(lower, ".zip") {
		return Image{}, fmt.Errorf("the link points to %q, not a .bin image or a .zip release", name)
	}
	// the version often sits in the folder name on firmware.grandstream.com
	// (Release_GRP261x_1.0.13.31/grp2610fw.bin), so look at the whole path
	if version == "" {
		if v, ok := ParseVersion(name); ok {
			version = v.Text
		} else if v, ok := ParseVersion(resp.Request.URL.Path); ok {
			version = v.Text
		}
	}
	tmpDir := filepath.Join(l.dir, ".download")
	os.MkdirAll(tmpDir, 0o700)
	tmp := filepath.Join(tmpDir, name)
	out, err := os.Create(tmp)
	if err != nil {
		return Image{}, err
	}
	total := resp.ContentLength
	var done int64
	buf := make([]byte, 256<<10)
	last := time.Now()
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				out.Close()
				os.Remove(tmp)
				return Image{}, werr
			}
			done += int64(n)
			if progress != nil && time.Since(last) > 300*time.Millisecond {
				progress(done, total)
				last = time.Now()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close()
			os.Remove(tmp)
			return Image{}, rerr
		}
	}
	out.Close()
	if progress != nil {
		progress(done, total)
	}
	defer os.Remove(tmp)
	img, err := l.AddFileAs(tmp, family, version)
	if err == nil {
		img.Source = u.Host
		l.mu.Lock()
		l.idx[img.File] = img
		l.saveLocked()
		l.mu.Unlock()
	}
	return img, err
}

func cleanName(n string) string {
	n = strings.ReplaceAll(n, " ", "_")
	if !safeFile.MatchString(n) {
		n = regexp.MustCompile(`[^A-Za-z0-9._+-]`).ReplaceAllString(n, "_")
	}
	return n
}

func (l *Library) Delete(file string) error {
	p, err := l.Path(file)
	if err != nil {
		return err
	}
	l.mu.Lock()
	delete(l.idx, file)
	l.saveLocked()
	l.mu.Unlock()
	return os.Remove(p)
}
