# Changelog

All notable changes to S3 Browser are listed here. Versions follow [Semantic Versioning](https://semver.org); dates are ISO 8601.

## [Unreleased]

## [1.2.1] - 2026-10-03

### Fixed
- The Flatpak build now shows the tray icon on GNOME-style panels: the sandbox uses the X11 socket and the window runs through XWayland, because the Wayland socket with fallback X11 gives the tray helper no display.

## [1.2.0] - 2026-10-03

### Added
- Interface in German, French, Spanish, Portuguese, Russian, Arabic (right-to-left) and Chinese, next to English and Turkish.
- Pull request and issue templates, code of conduct, support guide and this changelog.
- Desktop entry comments and the AppStream summary in every interface language.
- Installation table and data locations in the README; security notes for external editing, mounts and CloudFront.

### Changed
- Snap packaging was removed; Linux packages are AppImage, DEB, RPM, pacman and a Flatpak bundle.
- Moving objects between two connections that point at the same storage refuses the same location instead of deleting the object.
- Uploading a folder stops on an unreadable sub-folder instead of silently skipping it, which protected mirror syncs from deleting copies.
- Saving a text object uses a conditional write and keeps storage class, encryption, tags and public access.
- Objects above 5 GB are renamed and re-tagged with multipart server-side copies.
- Uploads from the external editor are retried, cancellations are reported and an unsynced copy is kept.
- Mounting explains why it is unavailable inside Flatpak or without FUSE.
- Releases are published only when the security audit passed.

## [1.1.0] - 2026-10-03

### Added
- Transfer queue with concurrency and bandwidth limits, pause, drag reordering, retry and cancellation.
- Parallel segmented downloads with MD5 verification of single-part objects.
- Mounting a bucket as a local folder (Linux with FUSE, macOS with macFUSE).
- Bucket settings: policy, ACL, CORS, lifecycle, tags, encryption, public access block, Object Lock, website hosting, logging, requester pays; CloudFront distributions on Amazon S3.
- Object settings: storage class, tags, metadata, ACL, Object Lock retention and legal hold, Glacier restore.
- Open with the default application and re-upload on save; download-direction folder sync; per-connection upload defaults; provider presets; file type icons; upload menu.

### Fixed
- Freezes with thousands of files, per-file notifications from mounts, lost encryption and access on in-place changes, misleading errors from providers without a setting.

## [1.0.0] - 2026-10-02

First release.

[Unreleased]: https://github.com/MehmetNuri/s3-browser/compare/v1.2.1...HEAD
[1.2.1]: https://github.com/MehmetNuri/s3-browser/compare/v1.2.0...v1.2.1
[1.2.0]: https://github.com/MehmetNuri/s3-browser/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/MehmetNuri/s3-browser/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/MehmetNuri/s3-browser/releases/tag/v1.0.0
