package firmware

import (
	"archive/zip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersion(t *testing.T) {
	v, ok := ParseVersion("Release_GRP261x_1.0.13.31.zip")
	if !ok || v.Text != "1.0.13.31" {
		t.Fatalf("%+v %v", v, ok)
	}
	a, _ := ParseVersion("1.0.11.74")
	b, _ := ParseVersion("1.0.9.135")
	if !a.Newer(b) || b.Newer(a) || a.Compare(a) != 0 {
		t.Fatal("compare")
	}
	if _, ok := ParseVersion("grp2610fw.bin"); ok {
		t.Fatal("no version expected")
	}
	for in, want := range map[string]string{"grp2610fw.bin": "grp2610", "GXP2130fw_1.0.11.74.bin": "gxp2130", "gxp1600fw-1.0.7.81.bin": "gxp1600", "wp822fw.bin": "wp822", "firmware.bin": "", "Release_GRP261x_1.0.13.31.zip": ""} {
		if got := FamilyFromFile(in); got != want {
			t.Errorf("FamilyFromFile(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLibrary(t *testing.T) {
	dir := t.TempDir()
	src := t.TempDir()
	bin := filepath.Join(src, "grp2610fw.bin")
	os.WriteFile(bin, []byte("firmware-bytes"), 0o644)
	l, err := OpenLibrary(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.AddFile(bin); err != ErrNeedVersion {
		t.Fatalf("expected ErrNeedVersion, got %v", err)
	}
	img, err := l.AddFileAs(bin, "", "1.0.13.31")
	if err != nil {
		t.Fatal(err)
	}
	if img.File != "grp2610fw-1.0.13.31.bin" || img.Family != "grp2610" || img.ServedName() != "grp2610fw.bin" {
		t.Fatalf("%+v", img)
	}
	// a release zip with a newer version
	zp := filepath.Join(src, "Release_GRP261x_1.0.13.40.zip")
	zf, _ := os.Create(zp)
	zw := zip.NewWriter(zf)
	w, _ := zw.Create("Release_GRP261x_1.0.13.40/grp2610fw.bin")
	w.Write([]byte("newer-bytes"))
	w2, _ := zw.Create("readme.txt")
	w2.Write([]byte("hi"))
	zw.Close()
	zf.Close()
	img2, err := l.AddFile(zp)
	if err != nil {
		t.Fatal(err)
	}
	if img2.Version.Text != "1.0.13.40" || img2.Source != "zip" {
		t.Fatalf("%+v", img2)
	}
	latest, ok := l.Latest("grp2610")
	if !ok || latest.File != img2.File {
		t.Fatalf("latest %+v", latest)
	}
	if len(l.List()) != 2 {
		t.Fatal("list")
	}
	// hand-copied file with the version in its name gets indexed on rescan
	os.WriteFile(filepath.Join(dir, "gxp2130fw_1.0.11.74.bin"), []byte("x"), 0o644)
	l.Rescan()
	if _, ok := l.Latest("gxp2130"); !ok {
		t.Fatal("rescan missed the file")
	}

	// the server
	var fetched []string
	s := NewServer(l, func(token, remote string, size int64) { fetched = append(fetched, token) })
	ts := httptest.NewServer(s)
	defer ts.Close()
	get := func(p string) (int, string) {
		resp, err := http.Get(ts.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	if code, body := get("/grp2610fw.bin"); code != 200 || body != "newer-bytes" {
		t.Fatalf("latest by family: %d %q", code, body)
	}
	tok := s.Offer(img.File)
	if code, body := get("/" + tok + "/grp2610fw.bin"); code != 200 || body != "firmware-bytes" {
		t.Fatalf("by token: %d %q", code, body)
	}
	if code, _ := get("/" + tok + "/gxp2130fw.bin"); code != 404 {
		t.Fatal("wrong family served under token")
	}
	if code, _ := get("/nope/grp2610fw.bin"); code != 404 {
		t.Fatal("unknown token served")
	}
	if code, _ := get("/../index.json"); code != 404 {
		t.Fatal("index served")
	}
	s.Withdraw(tok)
	if code, _ := get("/" + tok + "/grp2610fw.bin"); code != 404 {
		t.Fatal("withdrawn token served")
	}
	if len(fetched) != 2 || fetched[1] != tok {
		t.Fatalf("fetch hook %v", fetched)
	}

	if err := l.Delete(img.File); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, img.File)); err == nil {
		t.Fatal("file not deleted")
	}
	if err := l.Delete("../secret"); err == nil || !strings.Contains(err.Error(), "bad") {
		t.Fatal("escape accepted")
	}
}
