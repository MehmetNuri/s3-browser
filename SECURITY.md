# Security

Report security issues privately to **info@mehmetnuri.net**. Include the affected version, operating system, reproduction steps and expected impact. Avoid including real credentials or sensitive object contents.

## Local data

Secret access keys and session tokens in the local profile file are encrypted through Electron `safeStorage`, which uses the macOS Keychain, Windows DPAPI, or GNOME Keyring/KWallet on Linux. Existing plaintext values are encrypted the first time the profile file is read. On Linux systems without a usable keyring the values stay in plaintext, and the About dialog reports which mode is active. Access key IDs, endpoints and bucket names are not encrypted. Encryption protects the file at rest from other users and from backups; it does not protect against software running as the same user while the keyring is unlocked.

Profile and settings files are replaced atomically and use owner-only permissions on Unix; Windows access follows the user's filesystem permissions. JSON exports omit credentials, and imports do not overwrite existing profiles.

The renderer cannot pass local file paths to the backend: uploads start from a native dialog or from files the user dropped, whose paths are resolved in the preload script.

TLS verification is enabled unless a profile explicitly disables it. Use verification for remote storage endpoints.

## Downloads and transfers

Folder downloads use Go's rooted filesystem APIs to keep writes within the selected directory, including when symbolic links are present. Downloads are written to temporary files and replace the destination only after completion. Cancelling a transfer aborts its network context; retry starts the same operation again rather than resuming partial data.

## Releases

Release artifacts include SHA256 checksums. These detect changed downloads; desktop binaries are not currently code-signed or notarized. Report vulnerabilities against the current source or latest available release, and include the commit when using an Actions build.
