FROM debian:bookworm-slim

ENV WINDSURF_LS_PORT=42100 \
    WINDSURF_LS_INTERNAL_PORT=42101 \
    WINDSURF_LS_BINARY_PATH=/opt/windsurf/language_server_linux_x64 \
    WINDSURF_LS_API_SERVER_URL=https://server.self-serve.windsurf.com \
    WINDSURF_LS_REGISTER_USER_URL=https://api.codeium.com/register_user/ \
    WINDSURF_LS_CSRF_TOKEN=windsurf-api-csrf-fixed-token \
    WINDSURF_LS_DATA_DIR=/opt/windsurf/data \
    WINDSURF_LS_WORKSPACE_DIR=/tmp/windsurf-workspace \
    WINDSURF_LS_AUTO_SETUP=1

RUN apt-get update \
    && apt-get install -y --no-install-recommends bash curl ca-certificates netcat-openbsd socat tini \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY deploy/windsurf-ls/install-ls.sh /app/install-ls.sh
COPY deploy/windsurf-ls/docker-entrypoint.sh /app/docker-entrypoint.sh

RUN chmod +x /app/install-ls.sh /app/docker-entrypoint.sh \
    && mkdir -p /opt/windsurf/data/db /tmp/windsurf-workspace

VOLUME ["/opt/windsurf", "/tmp/windsurf-workspace"]

EXPOSE 42100

ENTRYPOINT ["/usr/bin/tini", "--", "/app/docker-entrypoint.sh"]
