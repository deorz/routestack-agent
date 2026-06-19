# Build stage
FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" -o /routestack-agent ./cmd/routestack-agent/

# Runtime stage — needs nftables and certbot installed; systemd accessed via host namespaces
FROM ubuntu:24.04 AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates certbot nftables && \
    rm -rf /var/lib/apt/lists/*
COPY --from=builder /routestack-agent /usr/local/bin/routestack-agent
# Create required directories
RUN mkdir -p /etc/routestack/agent /etc/routestack/services /etc/routestack/nftables \
    /etc/letsencrypt /var/lib/routestack/backups /var/lib/routestack/components \
    /var/log/routestack /run/routestack
ENTRYPOINT ["/usr/local/bin/routestack-agent"]
CMD ["run"]
