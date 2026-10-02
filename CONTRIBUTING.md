# Contributing

Use English for documentation, code comments, issues and pull requests. The application supports English and Turkish; keep both translation dictionaries in sync.

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
