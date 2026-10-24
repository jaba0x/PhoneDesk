# Your data

Everything PhoneDesk knows lives in one folder on your computer. There is no account and no cloud.

| OS | Default folder |
|---|---|
| macOS | `~/Library/Application Support/PhoneDesk` |
| Windows | `%AppData%\PhoneDesk` |

| What | Where |
|---|---|
| Phones, credentials, templates and settings | `inventory.json` |
| Encryption key for the passwords | `secret.key`, readable only by your user |
| The last reading of every phone, so the app opens with data | `snapshots/` |
| Saved copies of each phone's settings | `backups/<phone id>/` |
| Firmware images | `firmware/` |

Passwords in `inventory.json` are encrypted with AES-GCM using the key in `secret.key`. They're never shown in the app again after you enter them.

## Choosing another folder

**Settings → Data → Change…** picks a different folder: another disk, a synced drive, a shared folder. You choose whether to move the current data there, copy it, or open data that's already in that folder. The key moves with the data, so the same folder works from any computer that can reach it. **Use default** goes back.

A small `location.json` in the default folder remembers the choice. If the chosen folder isn't available when PhoneDesk starts (an unplugged drive, say), it opens the default folder and tells you.

## Moving to another computer

**Settings → Data → Export…** writes a single `.pdbak` file with everything, optionally including the saved copies and last readings. Passwords inside are re-encrypted with a passphrase you choose (Argon2id for the key, AES-GCM for the data), so the file is safe to carry around and opens on any computer with that passphrase.

On the other computer, **Import…** opens the file and either adds its phones and templates to what's already there (phones that already exist are skipped) or replaces everything with the file's contents.

## What leaves your computer

- Requests to your phones' web interfaces.
- A request to `api.github.com` at start and every six hours, to see whether a newer PhoneDesk release is out.
- Downloads you start from the firmware library, from whatever link you pasted.

Nothing else.
