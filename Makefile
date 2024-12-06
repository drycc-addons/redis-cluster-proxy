all: build

build:
	@mkdir -p bin
	go build -o bin/valkey-cluster-proxy ./cmd

clean:
	@rm -rf bin

test:
	go test -v ./... -race
