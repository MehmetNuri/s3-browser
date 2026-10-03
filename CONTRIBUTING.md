# Contributing

Use English for documentation, code comments, issues and pull requests. The application is translated into nine languages (see README); keep all translation dictionaries in sync.

## Development

Follow the setup in [README.md](README.md). Before submitting a change, run:

```sh
(cd backend && go test -race ./...)
npm run check
npm run build
```

Format Go files with `gofmt`. Add tests for changes to storage operations, path handling, credentials or transfer lifecycle behaviour. Tests must use temporary files and fake S3 services rather than real accounts.

Use Svelte 5 runes and typed component props. Keep derived values in `$derived`, event-triggered operations in handlers, and subscriptions paired with cleanup. Dialogs should support keyboard navigation, focus restoration and Escape. Respect reduced-motion preferences when adding transitions.

## Pull requests

Explain the user-visible problem and the resulting behaviour, then describe relevant validation. Keep changes focused. Do not commit access keys, profile backups, generated binaries, `node_modules` or `frontend/dist`.

Packaging changes should cover dependency declarations, installation paths, desktop integration and artifact names. Distinguish a successful build or dependency check from a test on a real desktop.

## Branches and pull requests

`main` is protected: it only changes through pull requests, force pushes and deletions are refused, and the `check` and `audit` workflow jobs must pass before a merge. Releases are tagged `v*` on `main`; tags cannot be moved or deleted.

Branch names follow `<type>/<short-description>` with one of these types: `feature`, `fix`, `chore`, `docs`, `ci`, `release` (for example `fix/mirror-unreadable-folders`). Dependabot uses its own `dependabot/` prefix. Pull requests are squash-merged, so keep each one to a single topic and write the title as the commit message you want to keep; the branch is deleted after the merge.
