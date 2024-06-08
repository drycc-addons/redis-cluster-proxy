package proxy

import (
	"bufio"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/drycc-addons/redis-cluster-proxy/pool"
	resp "github.com/drycc-addons/redis-cluster-proxy/proto"
	"github.com/golang/glog"
)

type RedisProxy struct {
	pools        map[string]pool.Pool
	maxIdle      int
	connTimeout  time.Duration
	mu           sync.Mutex
	password     string
	sendReadOnly bool
}

func NewRedisProxy(maxIdle int, connTimeout time.Duration, password string, sendReadOnly bool) *RedisProxy {
	p := &RedisProxy{
		pools:        make(map[string]pool.Pool),
		maxIdle:      maxIdle,
		password:     password,
		connTimeout:  connTimeout,
		sendReadOnly: sendReadOnly,
	}
	return p
}

func (cp *RedisProxy) Get(server string) (net.Conn, error) {
	var err error
	cp.mu.Lock()
	p := cp.pools[server]
	// create a pool is quite cheap and will not triggered many times
	if p == nil {
		p, err = pool.NewChannelPool(0, cp.maxIdle, func() (net.Conn, error) {
			return cp.postConnect(net.DialTimeout("tcp", server, cp.connTimeout))
		})
		if err != nil {
			glog.Fatal(err)
		}
		cp.pools[server] = p
	}
	cp.mu.Unlock()
	return p.Get()
}

func (cp *RedisProxy) Auth(password string) bool {
	return cp.password == password
}

func (cp *RedisProxy) postConnect(conn net.Conn, err error) (net.Conn, error) {
	if cp.password != "" {
		cmd, _ := resp.NewCommand("AUTH", cp.password)
		if err := cp.requestCommand(cmd, conn); err != nil {
			return conn, err
		}
	}

	if err != nil || !cp.sendReadOnly {
		return conn, err
	}
	defer func() {
		if err != nil {
			conn.Close()
			conn = nil
		}
	}()
	return conn, cp.requestCommand(REDIS_CMD_READ_ONLY, conn)
}

func (cp *RedisProxy) requestCommand(command *resp.Command, conn net.Conn) error {
	if _, err := conn.Write(command.Format()); err != nil {
		glog.Errorf("write %s failed, addr: %s, error: %s", command.Name(), conn.RemoteAddr().String(), err)
		return err
	}

	var data *resp.Data
	reader := bufio.NewReader(conn)
	data, err := resp.ReadData(reader)
	if err != nil {
		glog.Errorf("read %s resp failed, addr: %s, error: %s", command.Name(), conn.RemoteAddr().String(), err)
		return err
	}

	if data.T == resp.T_Error {
		glog.Error("%s resp is not OK, addr: %s", command.Name(), conn.RemoteAddr().String())
		return fmt.Errorf("post connect error: %s resp is not OK", command.Name())
	}
	return nil
}

func (cp *RedisProxy) Remove(server string) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	p := cp.pools[server]
	if p != nil {
		p.Close()
		delete(cp.pools, server)
	}
}
