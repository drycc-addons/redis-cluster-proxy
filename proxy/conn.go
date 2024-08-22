package proxy

import (
	"bufio"
	"fmt"
	"net"
	"time"

	resp "github.com/drycc-addons/redis-cluster-proxy/proto"
	"github.com/golang/glog"
)

type RedisConn struct {
	connTimeout  time.Duration
	password     string
	sendReadOnly bool
}

func NewRedisConn(maxIdle int, connTimeout time.Duration, password string, sendReadOnly bool) *RedisConn {
	p := &RedisConn{
		password:     password,
		connTimeout:  connTimeout,
		sendReadOnly: sendReadOnly,
	}
	return p
}

func (cp *RedisConn) Conn(server string) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", server, cp.connTimeout)
	if err != nil {
		return nil, err
	}
	return cp.postConnect(conn)
}

func (cp *RedisConn) Auth(password string) bool {
	return cp.password == password
}

func (cp *RedisConn) postConnect(conn net.Conn) (net.Conn, error) {
	if cp.password != "" {
		cmd, _ := resp.NewCommand("AUTH", cp.password)
		if _, err := cp.Request(cmd, conn); err != nil {
			defer conn.Close()
			return nil, err
		}
	}

	if _, err := cp.Request(REDIS_CMD_READ_ONLY, conn); err != nil {
		defer conn.Close()
		return nil, err
	}
	return conn, nil
}

func (cp *RedisConn) Request(command *resp.Command, conn net.Conn) (*resp.Data, error) {
	if _, err := conn.Write(command.Format()); err != nil {
		glog.Errorf("write %s failed, addr: %s, error: %s", command.Name(), conn.RemoteAddr().String(), err)
		return nil, err
	}

	var data *resp.Data
	reader := bufio.NewReader(conn)
	data, err := resp.ReadData(reader)
	if err != nil {
		glog.Errorf("read %s resp failed, addr: %s, error: %s", command.Name(), conn.RemoteAddr().String(), err)
		return nil, err
	}

	if data.T == resp.T_Error {
		glog.Errorf("%s resp is not OK, addr: %s, msg: %s", command.Name(), conn.RemoteAddr().String(), data.String)
		return nil, fmt.Errorf("post connect error: %s resp is not OK", command.Name())
	}
	return data, nil
}
