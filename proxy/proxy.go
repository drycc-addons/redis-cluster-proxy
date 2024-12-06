package proxy

import (
	"net"
	"sync"

	"github.com/golang/glog"
)

type Proxy struct {
	addr       string
	pool       *sync.Pool
	dispatcher *Dispatcher
	valkeyConn *ValkeyConn
	exitChan   chan struct{}
}

func NewProxy(addr string, dispatcher *Dispatcher, valkeyConn *ValkeyConn) *Proxy {
	p := &Proxy{
		addr: addr,
		pool: &sync.Pool{
			New: func() interface{} {
				return &Session{
					cached:      make(map[string]map[string]string),
					backQ:       make(chan *PipelineResponse, 1000),
					closeSignal: &sync.WaitGroup{},
					reqWg:       &sync.WaitGroup{},
					valkeyConn:  valkeyConn,
					dispatcher:  dispatcher,
					rspHeap:     &PipelineResponseHeap{},
				}
			},
		},
		dispatcher: dispatcher,
		valkeyConn: valkeyConn,
		exitChan:   make(chan struct{}),
	}
	return p
}

func (p *Proxy) Exit() {
	close(p.exitChan)
}

func (p *Proxy) handleConnection(cc net.Conn) {
	session := p.pool.Get().(*Session)
	defer session.Close()
	defer p.pool.Put(session)
	session.Reset(cc)
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
