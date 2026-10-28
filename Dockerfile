# syntax=docker/dockerfile:1

FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json ./
RUN npm install
COPY web ./
RUN npm run build

FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server ./cmd/server \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/agent ./cmd/agent

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/server /server
COPY --from=build /out/agent /agent-bin/agent-linux-amd64
COPY --from=web /web/dist /ui
ENV FLEET_HTTP_ADDR=:8080
ENV FLEET_DB_PATH=/data/fleet.db
ENV FLEET_UI_DIR=/ui
ENV FLEET_AGENT_BIN_DIR=/agent-bin
EXPOSE 8080
ENTRYPOINT ["/server"]
