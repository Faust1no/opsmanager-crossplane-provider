# Stage 1 — build the static provider binary.
# go mod vendor must be run before building this image so the local SDK
# replace directive (../go-client-mongodb-ops-manager) is resolved into vendor/.
FROM golang:1.21-alpine AS builder

WORKDIR /workspace

COPY go.mod go.sum ./
COPY vendor/ vendor/
COPY apis/ apis/
COPY cmd/ cmd/
COPY internal/ internal/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -mod=vendor -a \
    -ldflags="-s -w" \
    -o /provider \
    ./cmd/provider/main.go

# Stage 2 — minimal runtime image.
# Replace with your internal mirror if gcr.io is not reachable in the build environment.
FROM gcr.io/distroless/static:nonroot

COPY --from=builder /provider /provider

USER nonroot:nonroot
ENTRYPOINT ["/provider"]
