# Troubleshooting

## The phone shows as offline but I can open its web page

PhoneDesk checks the port it was told to use: 443 for HTTPS, 80 for HTTP, or whatever you entered. If the phone only has HTTP web access on and PhoneDesk is set to HTTPS, it looks offline. Edit the phone and set the scheme to **auto** (tries both) or to the one the phone has on.

## "Web login failed"

The admin password was refused. On a fresh phone it's `admin`, and some newer GRP models ship with a random password printed on a sticker on the back. Edit the credentials under Settings, or change the credential set the phone uses under Edit. If you changed `P2` through a template, the new password applies at once; update the credentials to match.

## "unexpected answer from /cgi-bin/dologin (HTTP 403)"

GXP16xx and GXP21xx firmware refuses API calls that don't look like they come from the phone's own web page. PhoneDesk 0.1.1 sends the headers the phone expects; update if you see this.

## "The phone logged in but sent no session id"

Very old firmware doesn't have the JSON API PhoneDesk uses. Update the phone from its own web page first; everything since the GXP16xx and GXP21xx firmware of the last several years works.

## Every account says "unknown"

Registration state comes from the phone's line status, which it only shares when **Action URI support** is on. Turn it on in the phone's web UI (Settings → Remote control on GRP phones; older GXP models have it under Settings as well) and read the phone again. The phone page's Overview lists this under "Reading notes" when it's the reason.

## Values show blank or wrong

Firmware differs in which status keys it answers. Open the phone's **Raw output** tab: that's exactly what the phone sent back. Post it in [Discussions](https://github.com/jaba0x/PhoneDesk/discussions) or an issue with the model and firmware version, and the read can be fixed for that model.

## A template line is refused

The phone validates every value and rejects the whole batch when one is wrong, so nothing is half-applied. The error names the value where the phone said so. Common causes: a setting that doesn't exist on that model, an out-of-range number, or a time zone code the firmware doesn't know. Try the same line on one phone from its Console tab to narrow it down.

## A firmware update never downloads

With the default source, the phone downloads the image from your computer on the port shown in Settings (6970 unless you changed it). The phone has to reach that port: allow it through the firewall on your computer, and make sure the phone's network can route to yours. Over a VPN or zero trust access that only lets you in, not the phone out, use Grandstream's server or a server of your own instead. See [Firmware updates](firmware.md).

## The phones can't reach my computer at all

Over zero trust access or a one-way VPN the built-in firmware server never gets a download. Use the SSH relay in Settings → Where phones fetch firmware from: PhoneDesk copies the image to a server on the phones' network (your PBX, say) and points the phones there. GXP16xx phones have no firmware upload in their web UI, so a server they can fetch from is the only route for them; GRP phones also fetch, so the relay covers everything.

## The app says a newer version is available but I'd rather wait

**Later** hides the bar for that version. It comes back when the next one is released.

## Where are the logs?

PhoneDesk writes no log files. Each firmware job keeps its own log on the Firmware page, and every read keeps the phone's answers under Raw output. If the app itself misbehaves, run it from a terminal (`/Applications/PhoneDesk.app/Contents/MacOS/PhoneDesk`, or `PhoneDesk.exe` from a command prompt) and the backend prints there.

## Asking for help

[Discussions](https://github.com/jaba0x/PhoneDesk/discussions) for questions and ideas, [Issues](https://github.com/jaba0x/PhoneDesk/issues) for bugs. Include the phone model, firmware version and, when a read looks wrong, the Raw output.
