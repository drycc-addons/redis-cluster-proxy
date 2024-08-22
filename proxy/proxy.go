package proxy

import (
	"bufio"
	"net"
	"sync"

	"github.com/golang/glog"
)

type Proxy struct {
	addr       string
	dispatcher *Dispatcher
	redisConn  *RedisConn
	exitChan   chan struct{}
}

func NewProxy(addr string, dispatcher *Dispatcher, redisConn *RedisConn) *Proxy {
	p := &Proxy{
		addr:       addr,
		dispatcher: dispatcher,
		redisConn:  redisConn,
		exitChan:   make(chan struct{}),
	}
	return p
}

func (p *Proxy) Exit() {
	close(p.exitChan)
}

func (p *Proxy) handleConnection(cc net.Conn) {
	session := &Session{
		Conn:           cc,
		r:              bufio.NewReaderSize(cc, 1024*512),
		cached:         make(map[string]map[string]string),
		backQ:          make(chan *PipelineResponse, 1000),
		closeSignal:    &sync.WaitGroup{},
		reqWg:          &sync.WaitGroup{},
		redisConn:      p.redisConn,
		dispatcher:     p.dispatcher,
		rspHeap:        &PipelineResponseHeap{},
		backendServers: make(map[string]*BackendServer),
	}
	session.Run()
}

func (p *Proxy) Run() {
	tcpAddr, err := net.ResolveTCPAddr("tcp", p.addr)
	if err != nil {
		glog.Fatal(err)
	}

	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		glog.Fatal(err)
	} else {
		glog.Infof("proxy listens on %s", p.addr)
	}
	defer listener.Close()

	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			glog.Error(err)
			continue
		}
		glog.Infof("accept client: %s", conn.RemoteAddr())
		go p.handleConnection(conn)
	}
}
