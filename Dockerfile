# Build both commands, ship neither the toolchain nor the source.
#
# `migrate` travels with `api` on purpose: the schema and the binary that
# expects it are one artifact, so a deploy cannot apply a migration from a
# different commit than the code it is for.

FROM golang:1.26-alpine AS build
WORKDIR /src

# Dependencies first, so a source-only change does not re-download them.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO off: the result runs on a bare image with no libc.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/seed ./cmd/seed

FROM alpine:3.20
# TLS roots, for talking to anything over https (an email provider, later).
RUN apk add --no-cache ca-certificates \
 && adduser -D -u 10001 waterloostar

WORKDIR /app
COPY --from=build /out/api /out/migrate /out/seed /usr/local/bin/
# `migrate` reads from ./migrations relative to its working directory.
COPY migrations ./migrations

# Never run as root: a container escape should not land on a privileged user.
USER waterloostar
EXPOSE 8080

# Migrations are deliberately NOT run here. A container that migrates on start
# races every other replica of itself; run it as its own step before rollout:
#     docker compose run --rm api migrate up
CMD ["api"]
