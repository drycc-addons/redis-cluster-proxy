package main

import (
	"flag"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/drycc-addons/redis-cluster-proxy/proxy"
	"github.com/golang/glog"
)

var config = struct {
	Addr                   string
	Password               string
	StartupNodes           string
	ConnectTimeout         time.Duration
	SlotsReloadInterval    time.Duration
	BackendIdleConnections int
	ReadPrefer             int
}{}

func init() {
	flag.StringVar(&config.Addr, "addr", "0.0.0.0:8088", "proxy serving addr")
	flag.StringVar(&config.Password, "password", "", "password for backend server, it will send this password to backend server")
	flag.StringVar(&config.StartupNodes, "startup-nodes", "127.0.0.1:7001", "startup nodes used to query cluster topology")
	flag.DurationVar(&config.ConnectTimeout, "connect-timeout", 3*time.Second, "connect to backend timeout")
	flag.DurationVar(&config.SlotsReloadInterval, "slots-reload-interval", 3*time.Second, "slots reload interval")
	flag.IntVar(&config.BackendIdleConnections, "backend-idle-connections", 5, "max number of idle connections for each backend server")
	flag.IntVar(&config.ReadPrefer, "read-prefer", proxy.READ_PREFER_MASTER, "where read command to send to, eg. READ_PREFER_MASTER, READ_PREFER_SLAVE, READ_PREFER_SLAVE_IDC")
}

func main() {
	flag.Parse()
	glog.Infof("%#v", config)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	glog.Infof("pid %d", os.Getpid())

	// shuffle startup nodes
	startupNodes := strings.Split(config.StartupNodes, ",")
	indexes := rand.Perm(len(startupNodes))
	for i, startupNode := range startupNodes {
		startupNodes[i] = startupNodes[indexes[i]]
		startupNodes[indexes[i]] = startupNode
	}
	redisProxy := proxy.NewRedisProxy(config.BackendIdleConnections, config.ConnectTimeout, config.Password, config.ReadPrefer != proxy.READ_PREFER_MASTER)
	dispatcher := proxy.NewDispatcher(startupNodes, config.SlotsReloadInterval, redisProxy, config.ReadPrefer)
	if err := dispatcher.InitSlotTable(); err != nil {
		glog.Fatal(err)
	}
	proxy := proxy.NewProxy(config.Addr, dispatcher, redisProxy)
	go proxy.Run()
	sig := <-sigChan
	glog.Infof("terminated by %#v", sig)
	proxy.Exit()
}
