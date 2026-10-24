package main

import (
	"context"
	"errors"
	"os/exec"
	goruntime "runtime"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/jaba0x/phonedesk/internal/core"
	"github.com/jaba0x/phonedesk/internal/firmware"
)

var errNoLibrary = errors.New("the firmware library folder couldn't be opened")

func (a *App) lib() (*firmware.Library, error) {
	if l := a.mgr.Library(); l != nil {
		return l, nil
	}
	return nil, errNoLibrary
}

// FirmwareImages lists the library, picking up files added by hand.
func (a *App) FirmwareImages() ([]firmware.Image, error) {
	l, err := a.lib()
	if err != nil {
		return nil, err
	}
	l.Rescan()
	return l.List(), nil
}

// AddResult reports files that were added and files that still need a
// version typed in (Grandstream's .bin files don't say which version they are).
type AddResult struct {
	Added       []firmware.Image `json:"added"`
	NeedVersion []string         `json:"needVersion"` // paths
	Errors      []string         `json:"errors"`
}

// AddFirmwareFiles copies chosen .bin images or release .zip files into the library.
func (a *App) AddFirmwareFiles() (AddResult, error) {
	res := AddResult{}
	l, err := a.lib()
	if err != nil {
		return res, err
	}
	paths, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "Add firmware",
		Filters: []runtime.FileFilter{{DisplayName: "Grandstream firmware (*.bin, *.zip)", Pattern: "*.bin;*.zip"}},
	})
	if err != nil || len(paths) == 0 {
		return res, err
	}
	for _, p := range paths {
		img, err := l.AddFile(p)
		if errors.Is(err, firmware.ErrNeedVersion) {
			res.NeedVersion = append(res.NeedVersion, p)
			continue
		}
		if err != nil {
			res.Errors = append(res.Errors, err.Error())
			continue
		}
		res.Added = append(res.Added, img)
	}
	a.emit("")
	a.syncRelayLater(res.Added)
	return res, nil
}

// AddFirmwareFileAs adds a file whose version (and family, if the name
// doesn't say) the user typed in.
func (a *App) AddFirmwareFileAs(path, family, version string) (firmware.Image, error) {
	l, err := a.lib()
	if err != nil {
		return firmware.Image{}, err
	}
	img, err := l.AddFileAs(path, family, version)
	if err == nil {
		a.emit("")
		a.syncRelayLater([]firmware.Image{img})
	}
	return img, err
}

var (
	dlMu     sync.Mutex
	dlCancel context.CancelFunc
)

// DownloadFirmware fetches an image or release zip from a link and reports
// progress with "firmware-download" events.
func (a *App) DownloadFirmware(link, family, version string) (firmware.Image, error) {
	l, err := a.lib()
	if err != nil {
		return firmware.Image{}, err
	}
	ctx, cancel := context.WithCancel(a.ctx)
	dlMu.Lock()
	if dlCancel != nil {
		dlMu.Unlock()
		cancel()
		return firmware.Image{}, errors.New("a download is already running")
	}
	dlCancel = cancel
	dlMu.Unlock()
	defer func() {
		dlMu.Lock()
		dlCancel = nil
		dlMu.Unlock()
		cancel()
	}()
	img, err := l.Download(ctx, link, family, version, func(done, total int64) {
		runtime.EventsEmit(a.ctx, "firmware-download", map[string]int64{"done": done, "total": total})
	})
	if err == nil {
		a.emit("")
		a.syncRelayLater([]firmware.Image{img})
	}
	if errors.Is(err, context.Canceled) {
		err = errors.New("cancelled")
	}
	return img, err
}

func (a *App) CancelDownload() {
	dlMu.Lock()
	defer dlMu.Unlock()
	if dlCancel != nil {
		dlCancel()
	}
}

func (a *App) DeleteFirmware(file string) error {
	l, err := a.lib()
	if err != nil {
		return err
	}
	err = l.Delete(file)
	a.emit("")
	return err
}

func (a *App) OpenFirmwareFolder() error {
	l, err := a.lib()
	if err != nil {
		return err
	}
	switch goruntime.GOOS {
	case "darwin":
		return exec.Command("open", l.Dir()).Start()
	case "windows":
		return exec.Command("explorer", l.Dir()).Start()
	}
	return exec.Command("xdg-open", l.Dir()).Start()
}

// ---- updates ----

// syncRelayLater copies freshly added images to the relay server in the
// background when that's the firmware source, reporting with "relay-sync" events.
func (a *App) syncRelayLater(imgs []firmware.Image) {
	if len(imgs) == 0 || a.st.Settings().FirmwareSource != "relay" {
		return
	}
	files := make([]string, 0, len(imgs))
	for _, img := range imgs {
		files = append(files, img.File)
	}
	go a.mgr.SyncRelay(a.ctx, files, func(file, state, msg string) {
		runtime.EventsEmit(a.ctx, "relay-sync", map[string]string{"file": file, "state": state, "message": msg})
	})
}

// SyncRelay copies the whole library (or the given files) to the relay server.
func (a *App) SyncRelay(files []string) error {
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Minute)
	defer cancel()
	return a.mgr.SyncRelay(ctx, files, func(file, state, msg string) {
		runtime.EventsEmit(a.ctx, "relay-sync", map[string]string{"file": file, "state": state, "message": msg})
	})
}

// RelayStates reports which images are on the relay server.
func (a *App) RelayStates() map[string]core.RelayState { return a.mgr.RelayStates() }

// StartUpgrade queues firmware updates; image "" picks the newest matching one.
func (a *App) StartUpgrade(ids []string, image string) ([]core.Job, error) {
	return a.mgr.StartUpgrade(ids, image)
}

func (a *App) UpgradeJobs() []core.Job { return a.mgr.Jobs() }

func (a *App) CancelUpgrade(id string) error { return a.mgr.CancelJob(id) }

func (a *App) ClearFinishedUpgrades() { a.mgr.ClearFinishedJobs() }

func (a *App) FirmwareServer() core.FirmwareServerInfo { return a.mgr.FirmwareServerInfo() }

func (a *App) StartFirmwareServer() error {
	err := a.mgr.StartFirmwareServer()
	a.emit("")
	return err
}

func (a *App) StopFirmwareServer() {
	a.mgr.StopFirmwareServer()
	a.emit("")
}
