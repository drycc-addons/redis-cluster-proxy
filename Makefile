all: build

build:
	@mkdir -p bin
	go build -o bin/redis-cluster-proxy ./cmd

clean:
	@rm -rf bin

test:
	go test -v ./... -race
