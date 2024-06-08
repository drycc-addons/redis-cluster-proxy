package proxy

import (
	"net"
	"time"

	"bufio"

	"math/rand"
	"strings"

	"github.com/drycc-addons/redis-cluster-proxy/pool"
	resp "github.com/drycc-addons/redis-cluster-proxy/proto"
	"github.com/golang/glog"
)

// dispatcher routes requests from all clients to the right backend
// it also maintains the slot table

const (
	// write commands always go to master
	// read from master
	READ_PREFER_MASTER = iota
	// read from slave if possible
	READ_PREFER_SLAVE
	// read from slave in the same idc if possible
	READ_PREFER_SLAVE_IDC

	CLUSTER_NODES_FIELD_NUM_IP_PORT = 1
	CLUSTER_NODES_FIELD_NUM_FLAGS   = 2
	// it must be larger than any FIELD index
	CLUSTER_NODES_FIELD_SPLIT_NUM = 4
)

var (
	REDIS_CMD_CLUSTER_SLOTS *resp.Command
	REDIS_CMD_CLUSTER_NODES *resp.Command
	REDIS_CMD_READ_ONLY     *resp.Command
)

func init() {
	REDIS_CMD_READ_ONLY, _ = resp.NewCommand("READONLY")
	REDIS_CMD_CLUSTER_NODES, _ = resp.NewCommand("CLUSTER", "NODES")
	REDIS_CMD_CLUSTER_SLOTS, _ = resp.NewCommand("CLUSTER", "SLOTS")
}

type Dispatcher struct {
	startupNodes       []string
	slotTable          *SlotTable
	slotReloadInterval time.Duration
	reqCh              chan *PipelineRequest
	redisProxy         *RedisProxy
	taskRunners        map[string]*TaskRunner
	// notify slots changed
	slotInfoChan   chan []*SlotInfo
	slotReloadChan chan struct{}
	readPrefer     int
}

func NewDispatcher(startupNodes []string, slotReloadInterval time.Duration, redisProxy *RedisProxy, readPrefer int) *Dispatcher {
	d := &Dispatcher{
		startupNodes:       startupNodes,
		slotTable:          NewSlotTable(),
		slotReloadInterval: slotReloadInterval,
		reqCh:              make(chan *PipelineRequest, 10000),
		redisProxy:         redisProxy,
		taskRunners:        make(map[string]*TaskRunner),
		slotInfoChan:       make(chan []*SlotInfo),
		slotReloadChan:     make(chan struct{}, 1),
		readPrefer:         readPrefer,
	}
	return d
}

func (d *Dispatcher) InitSlotTable() error {
	if slotInfos, err := d.reloadTopology(); err != nil {
		return err
	} else {
		for _, si := range slotInfos {
			d.slotTable.SetSlotInfo(si)
		}
	}
	return nil
}

func (d *Dispatcher) Run() {
	go d.slotsReloadLoop()
	for {
		select {
		case req, ok := <-d.reqCh:
			// dispatch req
			if !ok {
				glog.Info("exit dispatch loop")
				return
			}
			var server string
			if req.readOnly {
				server = d.slotTable.ReadServer(req.slot)
			} else {
				server = d.slotTable.WriteServer(req.slot)
			}
			taskRunner, ok := d.taskRunners[server]
			if !ok {
				glog.Info("create task runner", server)
				taskRunner = NewTaskRunner(server, d.redisProxy)
				d.taskRunners[server] = taskRunner
			}
			taskRunner.in <- req
		case info := <-d.slotInfoChan:
			d.handleSlotInfoChanged(info)
		}
	}
}

// remove unused task runner
func (d *Dispatcher) handleSlotInfoChanged(slotInfos []*SlotInfo) {
	newServers := make(map[string]bool)
	for _, si := range slotInfos {
		d.slotTable.SetSlotInfo(si)
		newServers[si.write] = true
		for _, read := range si.read {
			newServers[read] = true
		}
	}

	for server, tr := range d.taskRunners {
		if _, ok := newServers[server]; !ok {
			glog.Infof("exit unused task runner %s", server)
			tr.Exit()
			delete(d.taskRunners, server)
		}
	}
}

func (d *Dispatcher) Schedule(req *PipelineRequest) {
	d.reqCh <- req
}

// wait for the slot reload chan and reload cluster topology
// at most every slotReloadInterval
// it also reload topology at a relative long periodic interval
func (d *Dispatcher) slotsReloadLoop() {
	periodicReloadInterval := 60 * time.Second
	for range time.After(d.slotReloadInterval) {
		select {
		case _, ok := <-d.slotReloadChan:
			if !ok {
				glog.Infof("exit reload slot table loop")
				return
			}
			glog.Infof("request reload triggered")
			if slotInfos, err := d.reloadTopology(); err != nil {
				glog.Errorf("reload slot table failed")
			} else {
				d.slotInfoChan <- slotInfos
			}
		case <-time.After(periodicReloadInterval):
			glog.Infof("periodic reload triggered")
			if slotInfos, err := d.reloadTopology(); err != nil {
				glog.Errorf("reload slot table failed")
			} else {
				d.slotInfoChan <- slotInfos
			}
		}
	}
}

// request "CLUSTER SLOTS" to retrieve the cluster topology
// try each start up nodes until the first success one
func (d *Dispatcher) reloadTopology() (slotInfos []*SlotInfo, err error) {
	glog.Info("reload slot table")
	indexes := rand.Perm(len(d.startupNodes))
	for _, index := range indexes {
		if slotInfos, err = d.doReload(d.startupNodes[index]); err == nil {
			break
		}
	}
	return
}

/*
*
获取cluster slots信息，并利用cluster nodes信息来将failed的slave过滤掉
*/
func (d *Dispatcher) doReload(server string) (slotInfos []*SlotInfo, err error) {
	var conn net.Conn
	conn, err = d.redisProxy.Get(server)
	if err != nil {
		glog.Error(server, err)
		return
	} else {
		glog.Infof("query cluster slots from %s", server)
	}
	defer func() {
		if err != nil {
			conn.(*pool.PoolConn).MarkUnusable()
		}
		conn.Close()
	}()
	_, err = conn.Write(REDIS_CMD_CLUSTER_SLOTS.Format())
	if err != nil {
		glog.Errorf("write cluster slots error, server=%s, err=%v", server, err)
		return
	}
	r := bufio.NewReader(conn)
	var data *resp.Data
	data, err = resp.ReadData(r)
	if err != nil {
		glog.Error(server, err)
		return
	}
	slotInfos = make([]*SlotInfo, 0, len(data.Array))
	for _, info := range data.Array {
		slotInfos = append(slotInfos, NewSlotInfo(info))
	}

	// filter slot info with cluster nodes information
	_, err = conn.Write(REDIS_CMD_CLUSTER_NODES.Format())
	if err != nil {
		glog.Errorf("write cluster nodes error, server=%s, err=%v", server, err)
		return
	}
	r = bufio.NewReader(conn)
	data, err = resp.ReadData(r)
	if err != nil {
		glog.Error(server, err)
		return
	}
	aliveNodes := make(map[string]bool)
	lines := strings.Split(strings.TrimSpace(string(data.String)), "\n")
	for _, line := range lines {
		// 305fa52a4ed213df3ca97a4399d9e2a6e44371d2 10.4.17.164:7704 master - 0 1440042315188 2 connected 5461-10922
		glog.V(2).Info(line)
		elements := strings.SplitN(line, " ", CLUSTER_NODES_FIELD_SPLIT_NUM)
		glog.V(2).Info(len(elements), line)
		if !strings.Contains(elements[CLUSTER_NODES_FIELD_NUM_FLAGS], "fail") {
			aliveNodes[elements[CLUSTER_NODES_FIELD_NUM_IP_PORT]] = true
		} else {
			glog.Warningf("node fails: %s", elements[1])
		}
	}
	for _, si := range slotInfos {
		if d.readPrefer == READ_PREFER_MASTER {
			si.read = []string{si.write}
		} else if d.readPrefer == READ_PREFER_SLAVE || d.readPrefer == READ_PREFER_SLAVE_IDC {
			localIPPrefix := LocalIP()
			if len(localIPPrefix) > 0 {
				segments := strings.SplitN(localIPPrefix, ".", 3)
				localIPPrefix = strings.Join(segments[:2], ".")
				localIPPrefix += "."
			}
			var readNodes []string
			for _, node := range si.read {
				if !aliveNodes[node] {
					glog.Infof("filter %s since it's not alive", node)
					continue
				}
				if d.readPrefer == READ_PREFER_SLAVE_IDC {
					// ips are regarded as in the same idc if they have the same first two segments, eg 10.4.x.x
					if !strings.HasPrefix(node, localIPPrefix) {
						glog.Infof("filter %s by read prefer slave idc", node)
						continue
					}
				}
				readNodes = append(readNodes, node)
			}
			if len(readNodes) == 0 {
				readNodes = []string{si.write}
			}
			si.read = readNodes
		}
	}
	return
}

// schedule a reload task
// this call is inherently throttled, so that multiple clients can call it at
// the same time and it will only actually occur once
func (d *Dispatcher) TriggerReloadSlots() {
	select {
	case d.slotReloadChan <- struct{}{}:
	default:
	}
}
func (d *Dispatcher) Exit() {
	close(d.reqCh)
}
