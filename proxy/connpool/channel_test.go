package connpool

import (
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/rpc"
	"sync"
	"testing"
	"time"
)

var (
	InitCap    = 5
	MaxIdle    = 10
	MaximumCap = 100
	address    = "127.0.0.1:17777"
	factory    = func() (interface{}, error) {
		return rpc.DialHTTP("tcp", address)
	}
	closeFac = func(v interface{}) error {
		nc := v.(*rpc.Client)
		return nc.Close()
	}
)

func init() {
	// used for factory function
	port, err := getFreePort()
	if err != nil {
		panic(err)
	}
	address = fmt.Sprintf("127.0.0.1:%d", port)
	go rpcServer()
	time.Sleep(time.Millisecond * 1000) // wait until tcp server has been settled
}

func TestNew(t *testing.T) {
	p, err := newChannelPool()
	if err != nil {
		t.Errorf("New error: %s", err)
	}
	defer p.Release()
}
func TestPool_Get_Impl(t *testing.T) {
	p, _ := newChannelPool()
	defer p.Release()

	conn, err := p.Get()
	if err != nil {
		t.Errorf("Get error: %s", err)
	}
	_, ok := conn.(*rpc.Client)
	if !ok {
		t.Errorf("Conn is not of type poolConn")
	}
	p.Put(conn)
}

func TestPool_Get(t *testing.T) {
	p, _ := newChannelPool()
	defer p.Release()

	_, err := p.Get()
	if err != nil {
		t.Errorf("Get error: %s", err)
	}

	// after one get, current capacity should be lowered by one.
	if p.Len() != (InitCap - 1) {
		t.Errorf("Get error. Expecting %d, got %d",
			(InitCap - 1), p.Len())
	}

	// get them all
	var wg sync.WaitGroup
	for i := 0; i < (MaximumCap - 1); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := p.Get()
			if err != nil {
				t.Errorf("Get error: %s", err)
			}
		}()
	}
	wg.Wait()

	if p.Len() != 0 {
		t.Errorf("Get error. Expecting %d, got %d",
			(InitCap - 1), p.Len())
	}

	_, err = p.Get()
	if err != nil {
		t.Errorf("Get error: %s", err)
	}

}

func TestPool_Put(t *testing.T) {
	pconf := Config{InitCap: InitCap, Factory: factory, Close: closeFac, IdleTimeout: time.Second * 20,
		MaxIdle: MaxIdle}
	p, err := NewChannelPool(&pconf)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Release()

	// get/create from the pool
	conns := make([]interface{}, MaximumCap)
	for i := 0; i < MaximumCap; i++ {
		conn, _ := p.Get()
		conns[i] = conn
	}

	// now put them all back
	for _, conn := range conns {
		p.Put(conn)
	}

	if p.Len() != MaxIdle {
		t.Errorf("Put error len. Expecting %d, got %d",
			1, p.Len())
	}

	p.Release() // close pool

}

func TestPool_UsedCapacity(t *testing.T) {
	p, _ := newChannelPool()
	defer p.Release()

	if p.Len() != InitCap {
		t.Errorf("InitialCap error. Expecting %d, got %d", InitCap, p.Len())
	}
}

func TestPool_Close(t *testing.T) {
	p, _ := newChannelPool()

	// now close it and test all cases we are expecting.
	p.Release()

	c := p.(*channelPool)

	if c.conns != nil {
		t.Errorf("Close error, conns channel should be nil")
	}

	if c.factory != nil {
		t.Errorf("Close error, factory should be nil")
	}

	_, err := p.Get()
	if err == nil {
		t.Errorf("Close error, get conn should return an error")
	}

	if p.Len() != 0 {
		t.Errorf("Close error used capacity. Expecting 0, got %d", p.Len())
	}
}

func TestPoolConcurrent(t *testing.T) {
	p, _ := newChannelPool()
	pipe := make(chan interface{})

	go func() {
		p.Release()
	}()

	for i := 0; i < MaximumCap; i++ {
		go func() {
			conn, _ := p.Get()

			pipe <- conn
		}()

		go func() {
			conn := <-pipe
			if conn == nil {
				return
			}
			p.Put(conn)
		}()
	}
}

func TestPoolWriteRead(t *testing.T) {
	//p, _ := NewChannelPool(0, 30, factory)
	p, _ := newChannelPool()
	conn, _ := p.Get()
	cli := conn.(*rpc.Client)
	var resp int
	err := cli.Call("Arith.Multiply", Args{1, 2}, &resp)
	if err != nil {
		t.Error(err)
	}
	if resp != 2 {
		t.Error("rpc.err")
	}
}

func TestPoolConcurrent2(t *testing.T) {
	//p, _ := NewChannelPool(0, 30, factory)
	p, _ := newChannelPool()

	var wg sync.WaitGroup

	go func() {
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(i int) {
				conn, _ := p.Get()
				time.Sleep(time.Millisecond * time.Duration(rand.Intn(100)))
				p.Close(conn)
				wg.Done()
			}(i)
		}
	}()

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			conn, _ := p.Get()
			time.Sleep(time.Millisecond * time.Duration(rand.Intn(100)))
			p.Close(conn)
			wg.Done()
		}(i)
	}

	wg.Wait()
}

func newChannelPool() (Pool, error) {
	pconf := Config{
		InitCap:     InitCap,
		MaxIdle:     MaxIdle,
		Factory:     factory,
		Close:       closeFac,
		IdleTimeout: time.Second * 20,
	}
	return NewChannelPool(&pconf)
}

func rpcServer() {
	arith := new(Arith)
	rpc.Register(arith)
	rpc.HandleHTTP()

	l, e := net.Listen("tcp", address)
	if e != nil {
		panic(e)
	}
	go http.Serve(l, nil)
}

type Args struct {
	A, B int
}

type Arith int

func (t *Arith) Multiply(args *Args, reply *int) error {
	*reply = args.A * args.B
	return nil
}

func getFreePort() (port int, err error) {
	var a *net.TCPAddr
	if a, err = net.ResolveTCPAddr("tcp", "localhost:0"); err == nil {
		var l *net.TCPListener
		if l, err = net.ListenTCP("tcp", a); err == nil {
			l.Close()
			return l.Addr().(*net.TCPAddr).Port, nil
		}
	}
	return
}
