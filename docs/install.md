# Install

Download the latest build from the [Releases page](https://github.com/jaba0x/PhoneDesk/releases/latest). There's one zip for macOS and one for Windows.

=== "macOS"

    1. Get `PhoneDesk-…-macos-universal.zip`. It runs on Apple Silicon and Intel.
    2. Unzip it and drag **PhoneDesk** into Applications.
    3. The app isn't notarized by Apple, so the first time right-click it and choose **Open**. Or run:

        ```sh
        xattr -dr com.apple.quarantine /Applications/PhoneDesk.app
        ```

=== "Windows"

    1. Get `PhoneDesk-…-windows-x64.zip`.
    2. Unzip it anywhere and run `PhoneDesk.exe`.
    3. If SmartScreen warns about an unknown publisher, choose **More info**, then **Run anyway**.

    WebView2 is already part of Windows 10 and 11, so nothing else is needed.

## Build it yourself

You need Go 1.23 or newer. On a Mac you also need the Xcode command line tools (`xcode-select --install`).

```sh
git clone https://github.com/jaba0x/PhoneDesk.git
cd PhoneDesk
./scripts/build-mac.sh          # macOS: builds, installs to /Applications and opens it
.\scripts\build-windows.ps1     # Windows: builds build\bin\PhoneDesk.exe
```

The scripts install the Wails CLI if it's missing, run the tests and build a release binary.

## Staying up to date

PhoneDesk checks GitHub for a newer release when it starts and every few hours after that. When one is out, a yellow bar at the top of the window links to the download. **Settings → Check for updates** does the same on demand. Your data is picked up by the new version as it is.

## What it needs on the network

PhoneDesk talks to each phone's web interface, on TCP 443 (HTTPS) or 80 (HTTP), from your computer. Nothing runs on the phones and no server is needed. Firmware updates are the one exception: by default the phone downloads the image from your computer, see [Firmware updates](firmware.md).
