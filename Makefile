.PHONY: build test lint clean

build:
	go build -o worker-pool-memory ./cmd/module

test:
	go test -race -count=1 ./...

lint:
	golangci-lint run

clean:
	rm -f worker-pool-memory
	rm -f cmd/module/module
