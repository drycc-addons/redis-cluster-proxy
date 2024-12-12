package proxy

import (
	"bufio"
	"runtime"
	"sync"
	"time"

	"github.com/drycc-addons/valkey-cluster-proxy/fnet"
	"github.com/golang/glog"
	"github.com/maurice2k/ultrapool"
)

type Proxy struct {
	addr       string
	workers    *ultrapool.WorkerPool
	dispatcher *Dispatcher
	valkeyConn *ValkeyConn
	exitChan   chan struct{}
}

func NewProxy(addr string, dispatcher *Dispatcher, valkeyConn *ValkeyConn) *Proxy {
	workers := ultrapool.NewWorkerPool(func(task ultrapool.Task) {
		task.(*Session).WritingLoop()
	})
	maxProcs := runtime.GOMAXPROCS(0)
	workers.SetNumShards(maxProcs * 2)
	workers.SetIdleWorkerLifetime(5 * time.Second)
	workers.Start()

	p := &Proxy{
		addr:       addr,
		workers:    workers,
		dispatcher: dispatcher,
		valkeyConn: valkeyConn,
		exitChan:   make(chan struct{}),
	}
	return p
}

func (p *Proxy) Exit() {
	defer p.workers.Stop()
	close(p.exitChan)
}

func (p *Proxy) handleConnection(cc fnet.Connection) {
	session := &Session{
		Conn:        cc,
		r:           bufio.NewReaderSize(cc, 1024*512),
		cached:      make(map[string]map[string]string),
		backQ:       make(chan *PipelineResponse, 1000),
		closeSignal: &sync.WaitGroup{},
		reqWg:       &sync.WaitGroup{},
		valkeyConn:  p.valkeyConn,
		dispatcher:  p.dispatcher,
		rspHeap:     &PipelineResponseHeap{},
	}
	session.Prepare()
	p.workers.AddTask(session)
	session.ReadingLoop()
	defer session.Close()
}

func (p *Proxy) Run() {
	server, err := fnet.NewServer(p.addr)
	if err != nil {
		glog.Fatal(err)
	}
	config := server.GetListenConfig()
	config.SocketDeferAccept = true
	config.SocketFastOpen = true
	config.SocketReusePort = true

	server.SetRequestHandler(p.handleConnection)
	server.Listen()
	server.Serve()
}
