## What changes

<!-- One paragraph: what the change does and why. Link the issue if there is one. -->

## How it was verified

- [ ] `cd backend && go test -race ./...`
- [ ] `npm run check`
- [ ] Tried in the packaged app (AppImage or the platform package), not only in `npm run dev`
- [ ] Both English and Turkish strings updated when the UI changed (other locales may follow)

## Checklist

- [ ] No credentials, generated packages or `node_modules` in the diff
- [ ] Documentation and `CHANGELOG`/release notes updated when user-visible
- [ ] Security constraints in `SECURITY.md` still hold (renderer isolation, path containment, allowlisted IPC)
