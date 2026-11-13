# Multi-stage build for a minimal ambience image.
# Build layers are arranged to keep dependency and compiler caches hot across
# normal source edits, while the final runtime image stays distroless.
FROM artifacts.home.romaine.life/dockerhub/library/golang:1.26-alpine@sha256:30b1e546a9c7fa3b747c2bdba3a0c7bdf3a02ebb4c5b8405d2f948d7e8aac4ec AS build
WORKDIR /src
ENV GOPROXY=https://artifacts.home.romaine.life/go/go,direct

COPY go.mod go.sum ./
RUN go mod download

COPY scripts/build-web-wasm.sh ./scripts/build-web-wasm.sh
COPY rngutil ./rngutil
COPY sim ./sim
COPY cmd/ambience ./cmd/ambience
COPY cmd/ambience-wasm ./cmd/ambience-wasm
RUN ./scripts/build-web-wasm.sh \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ambience ./cmd/ambience

FROM artifacts.home.romaine.life/gcr/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=build /out/ambience /ambience
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/ambience"]
