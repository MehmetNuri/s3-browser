# Frontend

Svelte 5 and TypeScript UI for the Electron desktop application.

From the repository root, run `npm ci`, `npm --prefix frontend ci`, then `npm run dev`. Run `npm run check` for type checking and `npm run build:frontend` for a production build.

API wrappers live in `src/api.ts`; the isolated Electron preload supplies the bridge. Keep English and Turkish translations in `src/i18n.svelte.ts` synchronized. Use Svelte runes, typed props and subscription cleanup. Respect keyboard navigation and reduced-motion preferences.
