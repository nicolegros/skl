.PHONY: fmt test build install lint ci

fmt:
	go fmt ./...

test:
	go test ./...

build:
	go build -o skills .

install:
	go install .

lint:
	golangci-lint run ./...

ci: fmt lint test build
