# Development

PhoneDesk is a Go program with a [Wails](https://wails.io) v2 window. The UI is plain HTML, CSS and JavaScript with no build step.

```sh
git clone https://github.com/jaba0x/PhoneDesk.git
cd PhoneDesk
go test ./internal/...
wails dev
```

`wails dev` needs the Wails CLI (`go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.2`) and, on a Mac, the Xcode command line tools. `scripts/build-mac.sh` and `scripts/build-windows.ps1` build release binaries the same way the GitHub workflow does.

## Working on the UI

Open `frontend/dev/preview.html` in a browser. `frontend/dev/mock.js` stands in for the Go backend with a small fleet of sample phones, so every screen, dialog and state can be looked at without a phone in reach. The app itself serves `frontend/dist`.

## Layout

```
main.go, app.go        the window and the methods the UI calls (window.go.main.App.*)
data.go                data folder, export and import
firmware.go            firmware library and update jobs, from the UI's side
update.go              release check against GitHub
internal/grandstream   the phone's web API client, P-value names, snapshot building, templates, cfg.xml
internal/core          polling, problem rules, templates applied to phones, network scan, update jobs
internal/store         inventory, encrypted credentials, snapshots, settings copies, bundles
internal/firmware      image library, version parsing, the HTTP server phones download from
internal/relay         copies images over SSH to a server the phones can reach, with host-key pinning
internal/appver        version comparison for the update check
frontend/dist          the UI
frontend/dev           browser preview with sample data
web                    the landing page
docs                   this guide (MkDocs)
```

## How the phone is read

Everything goes through the phone's own web interface, the same endpoints its web pages use:

- `POST /cgi-bin/dologin` with the admin login returns a session id.
- `POST /cgi-bin/api.values.get` with `request=P47:P35:…` returns the values, 50 keys per batch. Status keys such as `prog`, `mac` and `Pphone_model` come the same way.
- `POST /cgi-bin/api.values.post` writes P-values. The phone validates the batch as a whole.
- `GET /cgi-bin/api-get_line_status` returns each line's state, and `api-sys_operation?request=REBOOT` reboots. These remote-control endpoints need Action URI support on the phone.

`internal/grandstream/parse.go` turns the raw values into a snapshot and is deliberately tolerant: each fact is taken from the first of several keys that has something in it, because firmware differs in what it answers. Reads are recorded request by request and shown on the Raw output tab.

## Tests

`go test ./internal/...` covers value parsing, model to family mapping, template parsing and diffing, firmware version parsing, the store and the bundle format. A phone that's misread makes a good new test: copy its Raw output into a test and assert what the snapshot should say.

## Releases

Push a tag like `v0.2.0`, or run the **build** workflow from the Actions tab with a version. It builds the macOS universal app and the Windows exe, tests `internal/`, and publishes a release with both zips. The docs and landing page deploy to GitHub Pages on every push to `main` that touches `docs/`, `web/` or `mkdocs.yml`.
