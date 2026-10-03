# S3 Browser

A desktop client for Amazon S3, Supabase Storage, MinIO, Cloudflare R2 and other S3-compatible services. Built with Electron, Svelte 5, TypeScript and Go. The interface is available in nine languages, including right-to-left Arabic.

![Browsing a bucket](docs/screenshots/browser.png)

## Screenshots

| | |
| --- | --- |
| **Transfer queue** — concurrency and bandwidth limits, pause, reorder, retry ![Transfer queue](docs/screenshots/transfers.png) | **Bucket settings** — encryption, Object Lock, public access, versioning ![Bucket settings](docs/screenshots/bucket-settings.png) |
| **Lifecycle rules** — JSON editors for policy, CORS and lifecycle with examples ![Lifecycle rules](docs/screenshots/bucket-lifecycle.png) | **Object settings** — storage class, permissions, tags, metadata ![Object settings](docs/screenshots/object-settings.png) |
| **Mounted bucket** — the bucket as a folder in the file manager ![Mount](docs/screenshots/mount.png) | **Connections** — presets for many S3-compatible providers ![New connection](docs/screenshots/connection.png) |
| **Storage analyzer** — space by folder, type, age and class, duplicates ![Storage analyzer](docs/screenshots/analyzer.png) | **Backup jobs** — scheduled folder-to-bucket syncs ![Backup jobs](docs/screenshots/backups.png) |
| **Command palette** — Ctrl+K for connections, buckets, files and commands ![Command palette](docs/screenshots/command-palette.png) | **S3 capabilities** — which API calls your provider really supports ![S3 capabilities](docs/screenshots/capabilities.png) |
| **Image preview** — previews and object details in the side panel ![Image preview](docs/screenshots/image-preview.png) | **Versions** — list, download, restore and delete object versions ![Versions](docs/screenshots/versions.png) |
| **HTTP headers** — Content-Type, Cache-Control, Content-Disposition, Content-Encoding ![Edit headers](docs/screenshots/headers.png) | **Copy to another connection** — also across providers and accounts ![Copy to](docs/screenshots/copy-to.png) |
| **Folder sync** — upload or download direction, with optional mirroring ![Folder sync](docs/screenshots/sync.png) | **Upload menu** — files, folders or drag and drop ![Upload menu](docs/screenshots/upload-menu.png) |
| **Light theme** — follows the system theme ![Light theme](docs/screenshots/light-theme.png) | **Language menu** — nine languages, switchable at runtime ![Language menu](docs/screenshots/language-menu.png) |
| **Arabic, right-to-left** — the whole layout mirrors ![Arabic interface](docs/screenshots/arabic.png) | **Turkish interface** ![Turkish interface](docs/screenshots/turkish.png) |

All screenshots were taken against a local S3-compatible test server with demo data.

## Features

- Connection profiles and credential-free JSON profile import/export. Secret keys are encrypted with the operating system's credential store when one is available.
- Browse buckets and objects, create folders, upload (including drag and drop), download, rename and delete objects.
- Transfer queue: every transfer is listed as queued, running, done or failed; set how many run at once, cap the total transfer rate, pause and resume the queue, reorder waiting items by dragging, cancel what is waiting and retry what failed. Objects of 32 MB and more are downloaded as parallel byte ranges, and downloads are verified against the object's MD5 ETag when the object was stored in one part.
- Text and image previews, metadata, folder sizes and presigned links with a selectable lifetime.
- Search a bucket or folder including all subfolders.
- Folder sync in both directions: upload only the new and changed files of a local folder, or download the new and changed objects of a prefix, each optionally mirroring deletions to the other side.
- Backup jobs: saved folder-to-bucket syncs that run on request or on a schedule while the application is open; each job is add-only or mirror, as chosen by the user.
- Object versions: list, download, restore and delete versions, and enable or suspend bucket versioning.
- Find and remove incomplete multipart uploads that silently take up storage.
- Desktop notification when transfers finish while the window is in the background.
- Edit HTTP headers (Content-Type, Cache-Control, Content-Disposition, Content-Encoding) of existing objects.
- Object settings: storage class (also for a selection), tags, user metadata, full ACL grants, Object Lock retention and legal hold, and Glacier restore requests.
- Bucket settings: policy, ACL grants, CORS rules, lifecycle rules, tags, default encryption, public access block, Object Lock, versioning, static website hosting, access logging and requester pays; CloudFront distributions of the bucket with cache invalidation on Amazon S3.
- Open an object with the desktop's default application; the copy is uploaded again whenever it is saved.
- Mount a bucket or folder as a local folder (Linux through FUSE, macOS with macFUSE): browse and read in the file manager, and with read-write access saved files are uploaded when closed. Not available on Windows.
- Per-connection defaults for the storage class and server-side encryption of new objects.
- Connection presets for Amazon S3, Supabase Storage, MinIO, Cloudflare R2, Backblaze B2, Wasabi, DigitalOcean Spaces, Hetzner, Scaleway, OVHcloud, Akamai/Linode, Exoscale, Storj, Google Cloud Storage (HMAC) and any other S3-compatible endpoint.
- Storage analyzer: space by folder, file type, age and storage class, the largest objects, and duplicate objects with the space they waste.
- Copy or move objects and folders to a bucket of another connection, including other providers and accounts.
- Edit small text objects in place, with a check that the object did not change meanwhile.
- Command palette (Ctrl+K) for connections, buckets, files and commands; favorite folders per connection.
- Light and dark themes that follow the system, keyboard shortcuts, accessible dialogs and reduced-motion animations.
- Multi-language support: English, Turkish, German, French, Spanish, Portuguese, Russian, Arabic and Chinese, with a right-to-left layout for Arabic. The language follows the system and can be changed at runtime; backend messages and the tray menu follow it.

## Installation

Download the file for your system from the [latest release](https://github.com/MehmetNuri/s3-browser/releases/latest); `SHA256SUMS` lists every checksum.

| System | File | Install |
| --- | --- | --- |
| Any Linux distribution | `S3_Browser-<version>-linux-x86_64.AppImage` | `chmod +x` and run; systems without FUSE can use `--appimage-extract-and-run` |
| Ubuntu, Debian, Pardus | `S3_Browser-<version>-linux-amd64.deb` | `sudo apt install ./S3_Browser-<version>-linux-amd64.deb` |
| Fedora, RHEL | `S3_Browser-<version>-linux-x86_64.rpm` | `sudo dnf install ./S3_Browser-<version>-linux-x86_64.rpm` |
| Arch Linux | `S3_Browser-<version>-linux-x64.pacman` | `sudo pacman -U S3_Browser-<version>-linux-x64.pacman` |
| Flatpak | `S3_Browser-<version>-linux-x86_64.flatpak` | `flatpak install ./S3_Browser-<version>-linux-x86_64.flatpak` (mounting a bucket is not available inside the Flatpak sandbox) |
| Windows | `S3_Browser-<version>-windows-x64-setup.exe` or `-portable.exe` | Run the installer, or the portable executable without installing |
| macOS, Apple Silicon | `S3_Browser-<version>-macos-arm64.dmg` | Open the DMG and drag the app to Applications; mounting needs [macFUSE](https://macfuse.github.io) |
| macOS, Intel | `S3_Browser-<version>-macos-x64.dmg` | Same as above |

The binaries are unsigned and the macOS builds are not notarized, so Windows SmartScreen and macOS Gatekeeper ask for confirmation on first launch.

### Where data is kept

| Data | Location |
| --- | --- |
| Connection profiles, settings, backup jobs | `~/.config/s3browser/` on Linux, `~/Library/Application Support/s3browser/` on macOS, `%AppData%\s3browser\` on Windows (About → Open config folder) |
| Mount points | `~/S3 Browser/<connection>-<bucket>/`, created on mount and removed on unmount |
| Copies opened with an external application | A private folder under the system temporary directory, removed when the application exits |

## Multi-language support

| Language | Code | Direction |
| --- | --- | --- |
| English | `en` | left-to-right |
| Türkçe | `tr` | left-to-right |
| Deutsch | `de` | left-to-right |
| Français | `fr` | left-to-right |
| Español | `es` | left-to-right |
| Português | `pt` | left-to-right |
| Русский | `ru` | left-to-right |
| العربية | `ar` | right-to-left |
| 中文 | `zh` | left-to-right |

The first language of the system that is on this list is used; the language menu at the bottom of the sidebar changes it at any time. Choosing Arabic mirrors the whole layout. Each language is one file under `frontend/src/locales/` (interface) and `backend/i18n_<code>.go` (messages from the Go backend); the TypeScript `Dict` type makes a missing key a compile error, and a backend test checks the Go message sets. The tray menu and transfer status words live in `desktop/tray-menu.cjs`.

## Supported platforms

Linux packages target Ubuntu, Debian/Pardus, Fedora, Red Hat Enterprise Linux and Arch Linux. Windows uses an installer or portable executable; macOS uses separate Intel and Apple Silicon DMGs. Electron bundles Chromium, so the application does not require WebKitGTK. Successful packaging does not replace testing on each desktop environment.

## System tray

| Desktop | Integration |
| --- | --- |
| GNOME | StatusNotifier/AppIndicator host, or an isolated X11 tray process for an existing Tray Icons Reloaded host. |
| KDE Plasma | StatusNotifierItem with a native context menu. |
| Windows | Notification area with an ICO icon. |
| macOS | Menu bar with a template icon and Retina variant. |

The tray menu provides Show window, About, Minimize to tray on close and Quit in the interface language. While transfers run, the tray icon turns into an animated arrow for the operation (upload, download, copy or move, sync), the tooltip and menu show progress such as "Uploading 12/340 · 42%", and the icon briefly shows a check mark or a warning when they finish. The taskbar entry shows the same progress where the desktop supports it. Closing exits when no tray host is detected. GNOME users need a compatible tray extension. The default Electron application menu is hidden.

On virtual machines the application uses software rendering. Set `S3BROWSER_DISABLE_GPU=1` to request it explicitly on other systems.

## Development

Use the Node.js LTS version in `.node-version` and Go specified in `backend/go.mod`. The Go backend lives in `backend/`, the Electron host in `desktop/` and the Svelte interface in `frontend/`. Electron and Svelte versions are pinned in their package manifests.

```sh
npm ci
npm --prefix frontend ci
npm run dev
```

The desktop process launches a local Go backend through private standard-input/output pipes. No local HTTP server is exposed. The renderer uses an isolated preload bridge with an API allowlist; Node.js integration is disabled and sandboxing is enabled.

```sh
(cd backend && go test -race ./...)
npm run check
npm run build
```

## Packages

```sh
npm run package:linux
npm run package:windows
npm run package:mac
```

Packages are written to `dist/`. Windows and macOS packages should be built on their respective platforms. `make appimage`, `make deb`, `make rpm` and `make archpkg` build individual Linux formats. RPM needs `rpmbuild`, pacman needs `bsdtar`, and the packaging tool bundled by electron-builder needs `libcrypt.so.1`; on Fedora and RHEL install it with `sudo dnf install libxcrypt-compat rpm-build bsdtar`. Flatpak target requires flatpak-builder and the runtimes.

Packaged builds disable Electron's `RunAsNode`, `NODE_OPTIONS` and `--inspect` entry points and load application code only from the ASAR archive.

To launch an AppImage, make it executable and run it. Systems without FUSE can use `--appimage-extract-and-run`.

## Releases

GitHub Actions builds packages on pushes and pull requests. Actions are pinned to commit hashes and updated through Dependabot. A `v*` tag matching the version in `package.json` publishes a release with every package (AppImage, DEB, RPM, pacman, Flatpak bundle, Windows installer and portable executable, macOS Intel and Apple Silicon DMGs) and a `SHA256SUMS` file. Desktop binaries are currently unsigned and macOS builds are not notarized.

## License

S3 Browser is licensed under the [Apache License 2.0](LICENSE). Bundled third-party components, such as Electron, the AWS SDK for Go and Remix Icon, keep their own licenses.

See [CONTRIBUTING.md](CONTRIBUTING.md), [SUPPORT.md](SUPPORT.md), [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md), [CHANGELOG.md](CHANGELOG.md) and [SECURITY.md](SECURITY.md). Contact: info@mehmetnuri.net.
