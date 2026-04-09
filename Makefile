VERSION ?= 0.2.0
BINARY = agent-memory
LDFLAGS = -s -w -X github.com/dklymentiev/agent-memory/cmd.Version=$(VERSION)

.PHONY: build test clean install

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

test:
	go test ./internal/... -count=1

test-verbose:
	go test ./internal/... -v -count=1

install: build
	cp $(BINARY) /usr/local/bin/

clean:
	rm -f $(BINARY)

lint:
	go vet ./...
