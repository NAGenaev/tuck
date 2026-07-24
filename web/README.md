# Tuck Web UI

React SPA for Tuck's web dashboard, built with Vite + Mantine. Its
application shell (theme, layout, sidebar, generic components) is forked
from [Remnawave](https://github.com/remnawave/frontend) — see `NOTICE` for
attribution and `LICENSE-AGPL-3.0` for the license governing this directory.

**This directory is licensed AGPL-3.0-only, distinct from the rest of the
Tuck repository (Apache-2.0).** See the top-level `README.md` for details.

## Develop

```sh
npm install
npm run dev        # dev server on :3333, proxies /v1, /v2, /openapi.json to a local Tuck server on :8200
```

## Build

```sh
npm run build       # writes to ../internal/ui/assets, embedded into the Go binary via go:embed
```

## Regenerate the typed API client

```sh
npm run gen:api      # reads ../internal/api/openapi.json, writes src/shared/api/schema.d.ts
```

Re-run whenever `internal/api/openapi.json` changes.
