# Analytics

Self-hosted, privacy-focused website analytics. No cookies, no fingerprinting,
no cross-site tracking. Each pageview is stored as path + referrer host +
timestamp — nothing tied to a visitor identity, no IP retained. The only cookie
anywhere is the admin's own first-party session.

**Status:** v0.1.0 — working backend, server-rendered minimal frontend.

## Stack

- **Backend:** Go (standard library + `net/http`, Go 1.22 routing)
- **Storage:** SQLite via `modernc.org/sqlite` (pure Go, no CGO)
- **Auth:** bcrypt (`golang.org/x/crypto`), signed-HMAC session cookie
- **Frontend:** server-rendered `html/template`, vanilla HTML/JS, no CSS
- **Deploy:** single static binary, multi-stage Docker, `docker compose`

## Project structure

```
analytics/
├── main.go              entrypoint, config, embed, helpers (rand, hmac)
├── web.go               routes, auth middleware, handlers, tracking collector
├── internal/
│   └── store/
│       ├── store.go     SQLite: schema, settings/auth, sites, events, stats
│       └── store_test.go unit tests (schema, auth, cascade, tz stats)
├── go.mod               deps: modernc sqlite + x/crypto
├── go.sum
├── Makefile             build / run / down / clean / test (all via Docker)
├── assets/
│   └── tracker.js       tracking snippet, served at /atag.js
├── templates/
│   ├── auth.html
│   ├── reset.html   	 forced change-password page
│   ├── dash.html 		 site list + create
│   └── site.html        per-site stats + snippet
├── Dockerfile           3-stage, CGO-free; final image is just the binary
├── compose.yaml         TZ / APP_NAME / SITE_DOMAIN inline
├── README.md
└── data/                created at runtime (gitignored)
    └── analytics.db     sites, events, password hash, session secret
```

`main`/`web` form the thin HTTP layer; persistence and stats live in the
`internal/store` package so they can be unit-tested in isolation. Templates and
`tracker.js` are compiled into the binary with `//go:embed`, so there are no
runtime file dependencies beyond the SQLite database.

## Features

### Tracking
- One static snippet for all sites; it reads its own `data-site` attribute and
  derives the collector endpoint from its `src` origin.
- Collector stores only path, referrer host, and timestamp.
- `navigator.sendBeacon` with a `fetch` fallback; `pushState` hooked for SPA
  route changes.
- Self-referrals and `www.` prefixes collapse to "Direct".

### Stats (per site)
- Visits today and visits last 7 days.
- Per-day breakdown for the last 7 days.
- Top pages and top referrers.
- Day boundaries computed in the configured timezone, not in SQL.

### Sites
- Create site → copy snippet → see traffic.
- List sites, view a site's stats, delete a site (cascades its events).

### Auth
- Single-admin password login, bcrypt-hashed in the database.
- Default password `admin`, with a forced change on first login before any
  dashboard access.
- Session secret persisted in the DB, so restarts don't invalidate sessions.

## Data model

```
settings(key TEXT PK, value TEXT)        -- password_hash, must_change, session_secret
sites(id TEXT PK, domain, created_at)
events(id, site_id, path, referrer, created_at)
  index: (site_id, created_at)
```

Events are append-only pageviews. There is no visitor table by design — counts
are pageviews, which keeps the system cookie-free and identity-free.

## Routes

| Method | Path                  | Auth | Purpose                       |
|--------|-----------------------|------|-------------------------------|
| GET    | `/dash`               | yes  | Dashboard (site list)         |
| GET    | `/auth`               | no   | Login form                    |
| POST   | `/auth`               | no   | Authenticate                  |
| POST   | `/logout`             | no   | Clear session                 |
| GET    | `/reset`              | yes  | Change-password form          |
| POST   | `/reset`              | yes  | Set new password              |
| POST   | `/sites`              | yes  | Create site                   |
| GET    | `/site/{id}`          | yes  | Per-site stats + snippet      |
| POST   | `/sites/{id}/delete`  | yes  | Delete site + events          |
| GET    | `/atag.js`            | no   | Tracking snippet              |
| POST   | `/api/event`          | no   | Collector (CORS open)         |
| OPTIONS| `/api/event`          | no   | CORS preflight                |

While `must_change` is set, every authed route redirects to `/reset`.

## Configuration

Set directly in `docker-compose.yml`:

| Variable      | Meaning                                                   |
|---------------|-----------------------------------------------------------|
| `TZ`          | IANA timezone for day boundaries (e.g. `Europe/Stockholm`)|
| `APP_NAME`    | Name shown in the dashboard                               |
| `SITE_DOMAIN` | Public base URL of this app; used to build the snippet    |

Also read from the environment if set: `PORT` (default `8080`), `DB_PATH`
(default `data/analytics.db`). The session cookie is marked `Secure` when
`SITE_DOMAIN` starts with `https://`.

## Build & run

```bash
# Docker
$EDITOR docker-compose.yml      # set the three values
docker compose up -d --build

# Local
go mod tidy
TZ=Europe/Stockholm go run .    # http://localhost:8080
```

First login uses password `admin`; you must set a new one before continuing.
Back up by copying `data/analytics.db`; delete it to reset (the default
password is re-seeded on next start).

## Roadmap

### v0.1.1 — React frontend
Replace the server-rendered templates with a SPA (React + shadcn/ui +
Tailwind CSS).

- Dockerfile gains a Node build stage: `npm ci && vite build` → `dist/`, then
  Go embeds `dist/` via `//go:embed`. Final image still ships only the binary,
  both toolchains discarded.
- Backend shifts from `html/template` to a JSON API; the four templates retire.
  `store.go` carries over unchanged.

### Later candidates
- Cookieless unique-visitor counting (daily-rotating salted hash).
- Configurable date ranges beyond today / last 7 days.
- `RESET_PASSWORD` flag to re-arm the forced-change flow without wiping data.
