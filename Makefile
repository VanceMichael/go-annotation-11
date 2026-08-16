GOTOOLCHAIN ?= local
export GOTOOLCHAIN

.PHONY: build test race vet fmt selfcheck docker clean

build:
	go build -o bin/ecoctl ./cmd/ecoctl

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -l .

selfcheck: build
	./bin/ecoctl selfcheck

docker:
	docker build -t ecoclaim:local .

clean:
	rm -rf bin
