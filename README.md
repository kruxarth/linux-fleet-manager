# Linux Fleet Manager

A web console for onboarding, monitoring, and running commands on a fleet of Linux machines.

Each machine runs a small Go agent that talks to the manager over HTTP, so machines behind NAT work too. The manager tracks which machines are online, runs a fixed set of safe commands on them, and feeds system metrics to Prometheus and Grafana.

## Features

- **Onboard machines** with a short-lived enrollment token.
- **Track availability.** A machine that stops sending heartbeats goes offline and stops accepting commands. The rest of the fleet keeps working.
- **Run allowlisted commands** such as `uptime` or `df -h`, with timeouts and an audit log. There is no free-form shell.
- **Graph metrics.** Prometheus scrapes `node_exporter` on each machine, and Grafana shows the dashboards.

## Project layout

```
cmd/server    Fleet Manager (API and web console)
cmd/agent     Agent that runs on each Linux machine
internal/     Core logic: store, node state, commands, HTTP
web/          React operator console
deploy/       Docker Compose, Prometheus, and Grafana config
dashboards/   Grafana dashboard JSON
```

## Quick start (Docker)

You need Docker with Compose v2.20 or newer.

```bash
docker compose up --build
```

The first build takes a few minutes. Then open:

- **Fleet Manager:** http://localhost:8080. Click **Sign in to the demo**. The admin token is `dev-admin-token`.
- **Grafana:** http://localhost:3000 (`admin` / `admin`)
- **Prometheus:** http://localhost:9090

The demo starts three fake machines. Each one is a Docker container running the real agent. From the **Fleet** page you can add, unplug, plug back in, and remove them.

To stop, run `docker compose down`. Add `-v` to also delete the fleet database.

> Demo mode mounts the Docker socket into the manager. That gives root-equivalent access to Docker, so only use it on a laptop. To turn it off, run `FLEET_DEMO_MODE=0 docker compose up --build`.

## Run locally (without Docker)

You need Go 1.25 or newer and Node.js 22 or newer.

```bash
export FLEET_ADMIN_TOKEN=dev-admin-token
make web          # build the console
make run-server   # manager on http://localhost:8080
```

Open the console, go to **Onboard**, and create an enrollment token. Then, in a second terminal, start an agent on this machine:

```bash
FLEET_ENROLL_TOKEN=<token> FLEET_MANAGER_URL=http://127.0.0.1:8080 make run-agent
```

To work on the UI with hot reload, run `cd web && npm install && npm run dev`. Vite serves on port 5173 and proxies `/api` to the manager.

Run the tests with `make test`.

## Configuration

| Variable | Default | What it does |
| --- | --- | --- |
| `FLEET_ADMIN_TOKEN` | required | Admin login token |
| `FLEET_HTTP_ADDR` | `:8080` | Address the manager listens on |
| `FLEET_DB_PATH` | `./data/fleet.db` | SQLite database file |
| `FLEET_ONLINE_AFTER` | `30s` | Heartbeat age that still counts as online |
| `FLEET_OFFLINE_AFTER` | `90s` | Heartbeat silence before a machine is offline |
| `FLEET_PROMETHEUS_URL` | unset | Shows scrape-target health in the console |
| `FLEET_DEMO_MODE` | `0` | Set to `1` to enable demo machines (Compose sets it to `1`) |
