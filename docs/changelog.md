# Changelog

## Unreleased

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
