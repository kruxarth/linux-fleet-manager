# Task: give the Fleet Manager console, Grafana and Prometheus the explainer's visual style

## Context

Repo: `~/coding/zoho`. It's a Linux fleet manager:
- Go control plane in `cmd/server` and `internal/`.
- Go node agent in `cmd/agent`.
- React + TypeScript + Vite operator console in `web/`, served by Go from `web/dist`.
- Prometheus and Grafana, run with `deploy/docker-compose.yml`.
- Grafana dashboards as JSON in `dashboards/`, provisioned from `deploy/grafana/provisioning/`.

Read `README.md` first.

The visual reference is `explainer/story.html`, an animated explainer the owner likes a lot. Open it in a browser and read its `<style>` block before designing anything. The goal: the tools the owner actually uses should feel like they come from the same world as that explainer.

## The design language to carry over (taken from story.html)

- **Palette:**
  - `--sky #E4ECF8`: page background, a cool pale blue.
  - `--ink #1F2A44`: text and outlines, navy.
  - `--soft #5C6B8A`: secondary text.
  - `--paper #FFFFFF`: surfaces.
  - `--mist #C9D6EC`: quiet fills and tracks.
- **Status colours:**
  - online `--leaf #1F9D63`
  - offline `--rose #E0484E`
  - pending `--sun #FFC94D`
- **Tool colours:**
  - Fleet Manager `--steel #3F68B5`
  - Prometheus `--ember #F07A1A`
  - Grafana `--plum #7559F2`
- **Type:**
  - Fredoka 500/600 for headings.
  - Nunito 500/700/800 for body text.
  - The system monospace stack for command names and values.
  - Self-host the fonts through `@fontsource/fredoka` and `@fontsource/nunito`. Don't use the Google CDN, because the console must work offline.
- **Shapes:**
  - Chunky 3px navy outlines.
  - Generous radii: 14–22px on cards, fully rounded pills for status.
  - A hard offset shadow (`0 5px 0 var(--ink)`) only on the single most important surface on a screen. Not on every card.
- **Characters:** the yellow node "faces" (happy / sick / asleep) and the little green agent with an antenna. Port the SVG from the `node()` helper in story.html into a React component, and use it on purpose:
  - as a node's avatar, with the face matching its state: online = happy, offline = asleep, error = sick
  - in empty states, e.g. "No nodes yet" plus a waving node plus the action to onboard one
  - on the login screen
  - Don't scatter characters as decoration elsewhere.
- **Words:** plain language from the operator's point of view, with the technical term next to it where it helps.
  - "Invite code (enrollment token)", "Last heartbeat 12s ago", "Runs only commands on the menu".
  - Sentence case. Buttons say exactly what they do.
- **Motion:**
  - Restrained: a heartbeat pulse on the online dot, a subtle transition when state changes.
  - Respect `prefers-reduced-motion`.

This is still an operator tool. It needs dense, scannable tables and clear status, not a cartoon. The playfulness comes from the palette, the outlines, the type and the node faces. Readability comes first.

## Scope, per tool (be realistic)

### 1. Fleet Manager console (`web/`): full redesign, main deliverable

- Replace the current "server aisle / 1U rack" theme in `web/src/styles.css` and the page components: Login, Fleet, NodeDetail, Enroll (shown as "Onboard"), Commands.
- **Keep every feature and API call working exactly as now.** Restyle and restructure the markup only. Don't change `web/src/api.ts` contracts or the Go API.
- **Fleet page:**
  - A top summary: counts online / offline / pending, each with its node face.
  - Then the node table: avatar, hostname, state pill, "last heartbeat Xs ago", kernel, arch, labels.
- **Node detail:**
  - Inventory.
  - State history, if the data exists.
  - The command menu as a visual "menu card", like Chapter 5 of the explainer.
  - Command output in a monospace panel.
  - The audit trail.
- **Onboard page:**
  - Show the three steps from Chapter 3 of the explainer: make invite code → run install command → node joins.
  - Give the install command a copy button.
  - Show the code's expiry and remaining uses.
- **Embed Grafana panels on the node detail page.** Use solo-panel iframes (`/d-solo/...&theme=light&kiosk`), which needs `GF_SECURITY_ALLOW_EMBEDDING=true` in compose. Anonymous viewer access is already on.
  - If Grafana can't be reached, show a friendly empty state instead of a broken frame. That matches the project's graceful-degradation story.
- Responsive down to tablet width. Visible keyboard focus. WCAG AA contrast. Check `--soft` on `--sky` and adjust if it fails.

### 2. Grafana: restyle what's actually under our control

You can't reskin Grafana's app chrome. Don't fork it or inject CSS. What you can do:
- Set `GF_USERS_DEFAULT_THEME=light` in compose so it sits closer to the palette.
- Rewrite `dashboards/linux-node.json` and `dashboards/fleet-overview.json` properly:
  - **Linux node:** a header row of stat or gauge panels (CPU busy %, memory used %, root disk used %, uptime), then time series below.
  - **Fleet overview:** stat panels for nodes online, offline and pending, then heartbeat-rate and command time series.
  - Correct units (percent, bytes, seconds).
  - Thresholds using leaf / sun / rose.
  - Fixed colours per series from the palette, not Grafana's default classic palette.
  - Short plain-language panel titles and descriptions.
  - Keep the template variables (node / instance) working.
  - Panel IDs and dashboard UIDs must stay stable, because the console embeds them.
- Optionally set the Grafana home dashboard to the fleet overview.

### 3. Prometheus: don't try to theme it

Its UI is compiled into the binary and can't be styled. Instead, bring the useful part into the console:
- Add a small "Metrics collection" status section that lists scrape targets and whether each is up.
- Do it through a thin, read-only Go endpoint, e.g. `GET /api/v1/metrics-status`. It queries Prometheus's `/api/v1/targets` at a new `FLEET_PROMETHEUS_URL` (default `http://prometheus:9090` in compose, unset = feature hidden).
- Admin-auth it like the other admin routes.
- Time out fast. Return a clear "Prometheus unreachable" state rather than an error page.
- Add a Go test for the handler.
- Keep this small. If it starts growing, stop and report back instead.

## Constraints

- Don't touch the agent, the enrollment or heartbeat logic, the command allowlist, or the DB schema.
- Don't add a UI framework or component library. Plain CSS (custom properties) plus React is enough.
- Keep the bundle lean: fonts plus your components, nothing else heavy.
- Update `README.md` where it describes the UI (the "1U rack" sentence) and document any new env vars.

## How to verify (do all of it, and report the results honestly)

1. `go test ./...` passes.
2. `cd web && npm run build` passes with no TypeScript errors.
3. `FLEET_ADMIN_TOKEN=dev-admin-token docker compose -f deploy/docker-compose.yml up --build -d`, then:
   - log in with `dev-admin-token`
   - create an invite code on Onboard
   - open a node's detail page
   - queue `uptime` on an online node and see the output
   - confirm the Grafana panels render embedded
   - stop Prometheus (`docker compose stop prometheus`) and confirm the console still works and shows the unreachable state.

   There's already one real enrolled node: the owner's own machine, hostname `krutarth-arch`. Use it, but don't revoke it.
4. Take headless screenshots of every console page and both Grafana dashboards at 1280×800 and 768×1024, for example with `google-chrome-stable --headless=new --screenshot`. **Look at them** and fix anything broken before you finish.
5. In your final report, list what changed per tool and include the screenshot paths. Call out anything you couldn't do.
