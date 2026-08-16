# Build a static binary, then ship it on nothing at all. The result is a
# ~10MB image with no shell, no package manager and no OS to patch.
FROM golang:1.22-alpine AS build

WORKDIR /src

# Dependencies first, so a code change does not re-download modules.
COPY go.mod go.sum ./
RUN go mod download

# Fetching from SCM is an outbound HTTPS call, and a scratch image has no trust
# store at all: without this the fetch fails with "certificate signed by unknown
# authority" while everything else keeps working.
RUN apk add --no-cache ca-certificates

COPY . .
RUN go vet ./... && go test ./...
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/psc-label .

FROM scratch

COPY --from=build /out/psc-label /psc-label

# The only file besides the binary: the CA bundle the SCM fetch verifies against.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# Setting PORT stops the program trying to open a browser, and is what Coolify
# routes to. Override it in Coolify if you need a different port.
ENV PORT=8080
EXPOSE 8080

# Nothing is written to disk, so the container needs no writable volume and no
# root. Its only outbound call is to SCM.
USER 65534:65534

# No shell in the image, so the binary health-checks itself.
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD ["/psc-label", "-healthcheck"]

ENTRYPOINT ["/psc-label"]
