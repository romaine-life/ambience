# Multi-stage build for a minimal ambience image.
# Build layers are arranged to keep dependency and compiler caches hot across
# normal source edits, while the final runtime image stays distroless.
FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY scripts/build-web-wasm.sh ./scripts/build-web-wasm.sh
COPY rngutil ./rngutil
COPY sim ./sim
COPY cmd/ambience ./cmd/ambience
COPY cmd/ambience-wasm ./cmd/ambience-wasm
RUN ./scripts/build-web-wasm.sh \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ambience ./cmd/ambience
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ambience-supervisor ./cmd/ambience-supervisor

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ambience /ambience
COPY --from=build /out/ambience-supervisor /ambience-supervisor
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/ambience"]
