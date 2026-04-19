# Multi-stage build for a minimal ambience-server image.
# Single statically-linked binary in a distroless base — ~15 MB total.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ambience-server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ambience-server /ambience-server
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/ambience-server"]
