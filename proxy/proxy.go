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
	redisProxy *RedisProxy
	exitChan   chan struct{}
}

func NewProxy(addr string, dispatcher *Dispatcher, redisProxy *RedisProxy) *Proxy {
	p := &Proxy{
		addr:       addr,
		dispatcher: dispatcher,
		redisProxy: redisProxy,
		exitChan:   make(chan struct{}),
	}
	return p
}

func (p *Proxy) Exit() {
	close(p.exitChan)
}

func (p *Proxy) handleConnection(cc net.Conn) {
	session := &Session{
		Conn:        cc,
		r:           bufio.NewReader(cc),
		backQ:       make(chan *PipelineResponse, 1000),
		closeSignal: &sync.WaitGroup{},
		reqWg:       &sync.WaitGroup{},
		redisProxy:  p.redisProxy,
		dispatcher:  p.dispatcher,
		rspHeap:     &PipelineResponseHeap{},
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

	go p.dispatcher.Run()

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
