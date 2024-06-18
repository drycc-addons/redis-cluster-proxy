package proxy

import (
	"bufio"
	"container/list"
	"errors"
	"io"
	"net"
	"time"

	"github.com/drycc-addons/redis-cluster-proxy/pool"
	resp "github.com/drycc-addons/redis-cluster-proxy/proto"
	"github.com/golang/glog"
)

var (
	errInitTaskRunnerConn = errors.New("init task runner connection error")
	errWriteToBackend     = errors.New("write to backend error")
	errRecoverFailed      = errors.New("try to recover from error failed")
)

const (
	TASK_CHANNEL_SIZE = 50000
)

/*
dispatcher------put req-------tr.in

tr.writer-------consume-------tr.in
                add to--------inflight
                write to------backend

tr.writer-------consume-------tr.out
                remove from---inflight
                put to--------backQ

tr.reader-------read from-----backend
                produce-------tr.out


为了保证request-response不会错匹配
1、读出错:
向tr.out<-error, 通知writer, 退出reader
writer处理到tr.out中的error时，进行recover
2、写出错：
进行recover

recover时，要
1、消费tr.in，使其不会阻塞
2、对所有request进行response
3、重置tr.out，起新的reader，关掉旧的backend connection让原来的reader退出

*/
// TaskRunner assure every request will be responded
type TaskRunner struct {
	in         chan interface{}
	out        chan interface{}
	inflight   *list.List
	server     string
	conn       net.Conn
	r          *bufio.Reader
	w          *bufio.Writer
	redisProxy *RedisProxy
	closed     bool
}

func NewTaskRunner(server string, redisProxy *RedisProxy) *TaskRunner {
	tr := &TaskRunner{
		in:         make(chan interface{}, TASK_CHANNEL_SIZE),
		out:        make(chan interface{}, TASK_CHANNEL_SIZE),
		inflight:   list.New(),
		server:     server,
		redisProxy: redisProxy,
	}

	if conn, err := redisProxy.Get(server); err != nil {
		glog.Error(tr.server, err)
	} else {
		tr.initRWConn(conn)
	}

	go tr.backingLoop()
	go tr.writingLoop()
	go tr.readingLoop(tr.out)

	return tr
}

func (tr *TaskRunner) readingLoop(out chan interface{}) {
	var err error
	defer func() {
		if err != io.EOF {
			glog.Error("exit reading loop", tr.server, err)
		}
		close(out)
	}()

	if tr.r == nil {
		err = errInitTaskRunnerConn
		out <- err
		return
	}
	// reading loop
	for {
		obj := resp.NewObject()
		if err = resp.ReadDataBytes(tr.r, obj); err != nil {
			out <- err
			return
		} else {
			out <- obj
		}
	}
}

func (tr *TaskRunner) backingLoop() {
	for rsp := range tr.out {
		if err := tr.handleResp(rsp); err != nil {
			if err := tr.tryRecover(err); err != nil {
				glog.Error("try recover err", err)
			}
		}
	}
}

func (tr *TaskRunner) writingLoop() {
	for req := range tr.in {
		if tr.closed && tr.inflight.Len() == 0 {
			// in queue和out queue都已经空了，writing loop可以退出了
			if tr.conn != nil {
				tr.conn.(*pool.PoolConn).MarkUnusable()
				tr.conn.Close()
			}
			close(tr.in)
			glog.Error("exit writing loop", tr.server)
			return
		}
		if err := tr.handleReq(req); err != nil {
			if err := tr.tryRecover(err); err != nil {
				glog.Error("try recover err", err)
			}
		}
	}
}

func (tr *TaskRunner) handleReq(req interface{}) error {
	var err error
	switch req := req.(type) {
	case *PipelineRequest:
		plReq := req
		err = tr.writeToBackend(plReq)
		if err != nil {
			glog.Error(errWriteToBackend, tr.server, err)
		}
	case struct{}:
		glog.Info("close task runner", tr.server)
		tr.closed = true
	}
	return err
}

func (tr *TaskRunner) handleResp(rsp interface{}) error {
	if tr.inflight.Len() == 0 {
		// this would occur when reader returned from blocking reading
		glog.Info("no inflight requests", rsp)
		if err, ok := rsp.(error); ok {
			return err
		} else {
			return nil
		}
	}

	plReq := tr.inflight.Remove(tr.inflight.Front()).(*PipelineRequest)
	plRsp := &PipelineResponse{
		ctx: plReq,
	}
	var err error
	switch rsp := rsp.(type) {
	case *resp.Object:
		plRsp.rsp = rsp
	case error:
		err = rsp
		plRsp.err = err
	}
	plReq.backQ <- plRsp
	return err
}

func (tr *TaskRunner) tryRecover(err error) error {
	tr.cleanupInflight(err)

	//try to recover
	if conn, err := tr.redisProxy.Get(tr.server); err != nil {
		tr.cleanupReqQueue()
		glog.Error(errRecoverFailed, tr.server, err)
		time.Sleep(100 * time.Millisecond)
		return err
	} else {
		glog.Info("recover success", tr.server)
		tr.initRWConn(conn)
		tr.resetOutChannel()
		go tr.readingLoop(tr.out)
	}

	return nil
}

func (tr *TaskRunner) cleanupInflight(err error) {
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

func (tr *TaskRunner) cleanupReqQueue() {
	for {
		select {
		case req := <-tr.in:
			tr.handleReq(req)
		default:
			return
		}
	}
}

func (tr *TaskRunner) writeToBackend(plReq *PipelineRequest) error {
	var err error
	// always put req into inflight list first
	tr.inflight.PushBack(plReq)

	if tr.w == nil {
		err = errInitTaskRunnerConn
		glog.Error(err)
		return err
	}
	buf := plReq.cmd.Format()
	if _, err = tr.w.Write(buf); err != nil {
		glog.Error(err)
		return err
	}
	if len(tr.in) == 0 {
		err = tr.w.Flush()
		if err != nil {
			glog.Error("flush error", err)
		}
	}
	return err
}

func (tr *TaskRunner) initRWConn(conn net.Conn) {
	if tr.conn != nil {
		tr.conn.(*pool.PoolConn).MarkUnusable()
		tr.conn.Close()
	}
	tr.conn = conn
	tr.r = bufio.NewReader(tr.conn)
	tr.w = bufio.NewWriter(tr.conn)
}

func (tr *TaskRunner) resetOutChannel() {
	tr.out = make(chan interface{}, TASK_CHANNEL_SIZE)
}

func (tr *TaskRunner) Exit() {
	tr.in <- struct{}{}
}
