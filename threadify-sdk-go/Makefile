.PHONY: all test lint fmt clean

all: lint test

test:
	go test -v ./...

lint:
	golangci-lint run

fmt:
	go fmt ./...

clean:
	go clean
	rm -f bin/*
