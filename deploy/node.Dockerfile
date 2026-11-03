# A pretend Linux node for local testing: the fleet agent plus the ordinary
# tools the command catalog calls (uname, uptime, df, free, whoami).
# The main image is distroless and has none of them.

FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -o /out/fleetmgr-agent ./cmd/agent

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends procps ca-certificates \
 && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/fleetmgr-agent /usr/local/bin/fleetmgr-agent
ENV FLEET_AGENT_DATA_DIR=/var/lib/fleetmgr-agent \
    FLEET_AGENT_CONFIG=/etc/fleetmgr/agent.yaml
ENTRYPOINT ["/usr/local/bin/fleetmgr-agent"]
