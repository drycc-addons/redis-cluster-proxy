# Redis Proxy

Redis Proxy is a lightweight, fast but powerful Redis Cluster Proxy written in Go.

## Usage

### Build

```bash
make build
```

### Command

```bash
# ./bin/redis-cluster-proxy --help
Usage of bin/redis-cluster-proxy:
  -addr string
         proxy serving addr (default "0.0.0.0:8088")
  -backend-idle-connections int
         max number of idle connections for each backend server (default 5)
  -connect-timeout duration
         connect to backend timeout (default 250ms)
  -debug-addr string
         proxy debug listen address for pprof and set log level, default not enabled
  -log-every-n uint
         output an access log for every N commands (default 100)
  -log-file string
         log file path (default "redis-cluster-proxy.log")
  -log-level string
         log level eg. debug, info, warn, error, fatal and panic (default "info")
  -password string
         password for backend server, it will send this password to backend server
  -read-prefer int
         where read command to send to, eg. READ_PREFER_MASTER, READ_PREFER_SLAVE, READ_PREFER_SLAVE_IDC
  -slots-reload-interval duration
         slots reload interval (default 3s)
  -startup-nodes string
         startup nodes used to query cluster topology (default "127.0.0.1:7001")
```

## Architecture

Each client connection is wrapped with a session, which spawns two goroutines to read request from and write response to the client. Each session appends it's request to dispatcher's request queue, then dispatcher route request to the right task runner according key hash and slot table. Task runner sends requests to its backend server and read responses from it.
Upon cluster topology changed, backend server will response MOVED or ASK error. These error is handled by session, by sending request to destination server directly. Session will trigger dispatcher to update slot info on MOVED error. When connection error is returned by task runner, session will trigger dispather to reload topology.

## Performance

Benchmark it with tests/bench/redis-benchmark.sh

## Development

The Drycc project welcomes contributions from all developers. The high-level process for development matches many other open source projects. See below for an outline.

* Fork this repository
* Make your changes
* [Submit a pull request][prs] (PR) to this repository with your changes, and unit tests whenever possible.
  * If your PR fixes any [issues][issues], make sure you write Fixes #1234 in your PR description (where #1234 is the number of the issue you're closing)
* Drycc project maintainers will review your code.
* After two maintainers approve it, they will merge your PR.

[prs]: https://github.com/drycc-addons/redis-cluster-proxy/pulls
[issues]: https://github.com/drycc-addons/redis-cluster-proxy/issues