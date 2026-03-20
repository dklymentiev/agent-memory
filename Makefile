VERSION ?= 0.1.0
BINARY = agent-memory
LDFLAGS = -s -w -X github.com/steamfoundry/agent-memory/cmd.Version=$(VERSION)

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
