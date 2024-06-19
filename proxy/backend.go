package proxy

import (
	"bufio"
	"container/list"
	"errors"
	"io"
	"net"
	"time"

	resp "github.com/drycc-addons/redis-cluster-proxy/proto"
	"github.com/golang/glog"
)

type BackendServer struct {
	inflight  *list.List
	server    string
	conn      net.Conn
	r         *bufio.Reader
	w         *bufio.Writer
	redisConn *RedisConn
}

func NewBackendServer(server string, redisConn *RedisConn) *BackendServer {
	tr := &BackendServer{
		inflight:  list.New(),
		server:    server,
		redisConn: redisConn,
	}

	if conn, err := redisConn.Conn(server); err != nil {
		glog.Error(tr.server, err)
	} else {
		tr.initRWConn(conn)
	}

	return tr
}

func (tr *BackendServer) Request(req *PipelineRequest) (*PipelineResponse, error) {
	if err := tr.writeToBackend(req); err != nil {
		glog.Error(err)
		if err := tr.tryRecover(err); err != nil {
			return nil, err
		}
		return nil, err
	}
	rsp := resp.NewObject()

	if err := resp.ReadDataBytes(tr.r, rsp); err != nil {
		glog.Error(err)
		if err := tr.tryRecover(err); err != nil {
			return nil, err
		}
		return nil, err
	}
	plReq := tr.inflight.Remove(tr.inflight.Front()).(*PipelineRequest)
	return &PipelineResponse{ctx: plReq, rsp: rsp}, nil
}

func (tr *BackendServer) writeToBackend(plReq *PipelineRequest) error {
	var err error
	// always put req into inflight list first
	tr.inflight.PushBack(plReq)

	if tr.w == nil {
		err = errors.New("init task runner connection error")
		glog.Error(err)
		return err
	}
	buf := plReq.cmd.Format()
	if _, err = tr.w.Write(buf); err != nil {
		glog.Error(err)
		return err
	}
	err = tr.w.Flush()
	if err != nil {
		glog.Error("flush error", err)
	}
	return err
}

func (tr *BackendServer) tryRecover(err error) error {
	tr.cleanupInflight(err)

	//try to recover
	if conn, err := tr.redisConn.Conn(tr.server); err != nil {
		glog.Error("try to recover from error failed", tr.server, err)
		time.Sleep(100 * time.Millisecond)
		return err
	} else {
		glog.Info("recover success", tr.server)
		tr.initRWConn(conn)
	}

	return nil
}

func (tr *BackendServer) cleanupInflight(err error) {
	for e := tr.inflight.Front(); e != nil; {
		plReq := e.Value.(*PipelineRequest)
		if err != io.EOF {
			glog.Error("clean up", plReq)
		}
		plRsp := &PipelineResponse{
			ctx: plReq,
			err: err,
		}
		plReq.backQ <- plRsp
		next := e.Next()
		tr.inflight.Remove(e)
		e = next
	}
}

func (tr *BackendServer) initRWConn(conn net.Conn) {
	if tr.conn != nil {
		tr.conn.Close()
	}
	tr.conn = conn
	tr.r = bufio.NewReader(tr.conn)
	tr.w = bufio.NewWriter(tr.conn)
}

func (tr *BackendServer) Close() {
	if tr.conn != nil {
		tr.conn.Close()
	}
}
