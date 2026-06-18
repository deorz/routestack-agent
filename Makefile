BINARY     := routestack-agent
CMD        := ./cmd/routestack-agent
LDFLAGS    := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)

.PHONY: build test lint vet fmt check clean install docker-build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(CMD)

build-linux-amd64:
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BINARY)-linux-amd64 $(CMD)

build-linux-arm64:
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BINARY)-linux-arm64 $(CMD)

test:
	go test -race -count=1 ./...

test-golden:
	go test -run Golden ./...

lint:
	golangci-lint run

vet:
	go vet ./...

fmt:
	gofmt -s -w .

check: vet lint test

clean:
	rm -f $(BINARY) $(BINARY)-linux-*

install: build
	cp $(BINARY) /usr/local/bin/

docker-build:
	docker build -t routestack-agent:$(VERSION) .
