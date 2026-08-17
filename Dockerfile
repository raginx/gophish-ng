# syntax=docker/dockerfile:1

# Build client-side assets (JS/CSS) with esbuild.
# Mirrors `npm run build` (build.js) - see README for why this replaced
# the old Gulp + Webpack setup.
FROM node:22-bookworm-slim AS build-js

WORKDIR /build
COPY package.json package-lock.json build.js ./
RUN npm ci
COPY static/js/src ./static/js/src
COPY static/css ./static/css
RUN node build.js


# Build the Go binary.
FROM golang:1.26-bookworm AS build-golang

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -v -o gophish .


# Runtime container.
FROM debian:stable-slim

LABEL org.opencontainers.image.title="gophish-ng" \
      org.opencontainers.image.description="Actively maintained, security-hardened fork of Gophish" \
      org.opencontainers.image.source="https://github.com/raginx/gophish-ng" \
      org.opencontainers.image.licenses="MIT"

RUN useradd -m -d /opt/gophish -s /bin/bash app

RUN apt-get update && \
	apt-get install --no-install-recommends -y jq libcap2-bin ca-certificates && \
	apt-get clean && \
	rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

WORKDIR /opt/gophish

# Pull in just what the running binary actually needs - not the whole
# build context (Go source, go.sum, etc.) like the old single `COPY
# --from=build-golang .../ ./` did.
COPY --from=build-golang /src/gophish ./
COPY --from=build-golang /src/VERSION ./
COPY --from=build-golang /src/config.json ./
COPY --from=build-golang /src/db ./db
COPY --from=build-golang /src/templates ./templates
COPY --from=build-golang /src/static ./static
COPY --from=build-golang /src/docker ./docker
COPY --from=build-js /build/static/js/dist ./static/js/dist
COPY --from=build-js /build/static/css/dist ./static/css/dist

RUN chown app:app config.json && chmod +x docker/run.sh

RUN setcap 'cap_net_bind_service=+ep' /opt/gophish/gophish

USER app
RUN sed -i 's/127.0.0.1/0.0.0.0/g' config.json && touch config.json.tmp

EXPOSE 3333 8080 8443 80

CMD ["./docker/run.sh"]
