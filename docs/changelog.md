# Changelog

## 0.1.4

- Delete works again: the library's delete button, Delete on phones, Remove on a phone's page, deleting templates and the "replace" import all asked with the browser's own confirm box, which the Mac window never shows, so the click quietly did nothing. They now ask with PhoneDesk's own dialog. Same for the version question after a download whose link doesn't say.
- GXP16xx and GXP21xx phones show their firmware, boot, core and base versions, MAC, address and uptime. Their status page answers with numbered keys (`P68` for the program version, `P67` for the MAC, and so on) instead of named ones, so the Firmware page said "No phones read yet" for a fleet of GXPs and never offered an update. Uptime comes from `api-get_time` when the phone doesn't answer the `uptime` key.
- Images added as "GXP1625" or "gxp1625" land under the right family (`gxp1600`), and an index with old entries is fixed when the library opens.
- With the SSH relay as the firmware source, an image added to the library (file, release zip or download) is copied to the relay server right away, so the update itself doesn't wait on the copy. The library shows which images are on the relay, with **Copy library to relay** for everything at once and a retry on failed copies.

## 0.1.3

- Choosing the SSH relay as the firmware source now sticks. 0.1.2 quietly reset it to "this computer" on save, so the Firmware page kept showing the old source.

## 0.1.2

- Firmware updates for networks where the phones can't reach your computer: a fourth source, "a server the phones can reach, filled over SSH". PhoneDesk copies the image over SSH to a server on the phones' network (a FreePBX box's `/tftpboot`, say) under `phonedesk/<version>/`, then points the phones at it over TFTP, HTTP or HTTPS. The copy is skipped when the same file is already there, the server's host key is pinned on first use, and **Test relay** in Settings checks the login and folder.

## 0.1.1

- GXP16xx and GXP21xx phones can be read: their firmware answers 403 Forbidden to API calls that don't carry the headers the phone's own web page sends (Referer, Origin), so PhoneDesk now sends them. A wrong password on these phones ("wrong3") is reported as a login failure.
- Every dashboard check can be switched off in Settings, grouped by area with its severity.
- An active account with nothing in it (the factory state of account 2 on many GXP phones) is no longer reported as having no SIP server; only an account with a user ID and no server is.
- Login works with firmware that sends the session id only as a cookie or as a plain string, HTTPS accepts TLS 1.0/1.1 and older cipher suites for old phones, and a login answer that isn't JSON is kept under Raw output with a clearer error.

## 0.1.0

First release.

- Dashboard with online/offline counts, registered accounts, and a problem list sorted by severity.
- Phones drawn with one LED per SIP account; live line state when Action URI support is on.
- Phone page: system facts, accounts, settings grouped by topic with their names, saved copies with compare and restore, a console for reading, writing and dialing, and raw output.
- Checks: offline, login failed, account not registered, account without a server, no active account, factory admin password, plain HTTP, Telnet and SSH open, no NTP, no syslog, DHCP option 66, newer firmware in the library, stale data, the same extension on two phones.
- Templates: `P-value = value` lines with a parameter picker, applied to many phones after a per-phone review, with an optional reboot. Export as `cfg<mac>.xml` provisioning files.
- Extensions table across all phones.
- Firmware library (files or Grandstream download links), updates one phone after another with the phone's upgrade settings restored afterwards, and a choice of source: this computer, Grandstream's server, or your own.
- Network scan, bulk add, search with state words, bulk actions.
- Data folder of your choosing, export and import with a passphrase, update check against GitHub releases.
