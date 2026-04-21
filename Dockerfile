# syntax=docker/dockerfile:1.7

# Multi-stage build for a minimal ambience image.
# Build layers are arranged to keep dependency and compiler caches hot across
# normal source edits, while the final runtime image stays distroless.
FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
	go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
	--mount=type=cache,target=/root/.cache/go-build \
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ambience ./cmd/ambience

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ambience /ambience
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/ambience"]
