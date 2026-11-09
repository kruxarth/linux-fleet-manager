# Decisions

## React operator console (not Go HTML templates)

UI aesthetics were not a priority in the original brief. The first plan used `html/template`. The product now uses a **React + TypeScript (Vite)** SPA because a real operator console is expected, and the Go server remains the only backend.

Production: Go serves `web/dist` and client-routes unknown GETs to `index.html`.
Development: Vite on `:5173` proxies `/api` to `:8080`.

## UI restyle (2026-11-07)
- The original design called for a "server aisle / 1U rack" console (Figtree + IBM Plex Mono). Superseded: the console now follows `explainer/story.html` (Fredoka + Nunito, navy outlines, node faces). Behaviour and API contracts are unchanged.
- Added read-only `GET /api/v1/metrics-status` (needs `FLEET_PROMETHEUS_URL`) so Prometheus's scrape health is visible in the console instead of trying to theme its compiled-in UI.

## Demo mode (2026-11-07)
- Goal: someone who has never seen the project runs `docker compose up --build` and can demo onboarding, offline detection and commands without touching a terminal again.
- Demo machines are Docker containers built from `deploy/node.Dockerfile`: the real agent plus `procps`, so every catalog command works. The main image is distroless and has no `uptime` or `free`. Each one enrolls through the normal path, with its own single-use invite code minted by the manager. Nothing in the enrollment, heartbeat or command code is special-cased.
- The manager drives Docker over the unix socket with plain `net/http` (`internal/demo`), so there's no Docker SDK dependency. It only ever acts on containers labelled `fleetmgr.demo=1`; any other name gets a 404.
- On shutdown the manager removes its demo containers and revokes their nodes, so `docker compose down` leaves nothing behind. On start it tops up to `FLEET_DEMO_NODES`.
- Rejected: a terminal in the web console. A browser shell on the host is exactly the "free-form remote shell" this project argues against. Buttons for fixed actions (add, unplug, plug in, remove) keep the allowlist spirit.
- Trade-off: mounting `/var/run/docker.sock` gives the manager root-equivalent access to Docker. Acceptable for a single-laptop demo, it's behind `FLEET_DEMO_MODE`, and it's off unless that is set.
- The server image is now built for the build machine's own architecture (it was hard-coded to amd64), so the demo also starts on Apple Silicon. The downloadable agent stays `agent-linux-amd64`.
- `install.sh` no longer relies on `hostname -I` (GNU-only, missing on Arch), which left the node_exporter address as a bare `:9100`.
