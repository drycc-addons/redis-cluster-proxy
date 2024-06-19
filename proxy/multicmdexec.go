package proxy

import (
	"fmt"
	"slices"

	resp "github.com/drycc-addons/redis-cluster-proxy/proto"
	"github.com/golang/glog"
)

type MultiCmdExec struct {
	session    *Session
	serverCmds map[string][]*resp.Command
}

func NewMultiCmdExec(session *Session) *MultiCmdExec {
	multiCmdExec := &MultiCmdExec{
		session:    session,
		serverCmds: make(map[string][]*resp.Command),
	}
	for _, subCmd := range *session.multiCmd {
		var server string
		if CmdReadOnly(subCmd) {
			server = session.dispatcher.slotTable.ReadServer(Key2Slot(subCmd.Value(1)))
		} else {
			server = session.dispatcher.slotTable.WriteServer(Key2Slot(subCmd.Value(1)))
		}
		multiCmdExec.serverCmds[server] = append(multiCmdExec.serverCmds[server], subCmd)
	}
	return multiCmdExec
}

func (m *MultiCmdExec) execServer(server string) (*resp.Data, error) {
	var err error
	var data *resp.Data
	conn, err := m.session.redisConn.Conn(server)
	defer func() {
		if err != nil {
			glog.Error(err)
		}
		conn.Close()
	}()
	if err == nil {
		cmd, _ := resp.NewCommand("MULTI")
		_, err = m.session.redisConn.Request(cmd, conn)
		if err == nil {
			for _, cmd := range m.serverCmds[server] {
				m.session.redisConn.Request(cmd, conn)
			}
			cmd, _ := resp.NewCommand("EXEC")
			data, err = m.session.redisConn.Request(cmd, conn)
		}
	}
	if err != nil || data == nil {
		return &resp.Data{T: resp.T_Error, String: []byte(fmt.Sprintf("error is: %v", err))}, err
	}
	return data, err
}

func (m *MultiCmdExec) Exec() (*resp.Data, error) {
	var err error
	data := &resp.Data{T: resp.T_Array, Array: make([]*resp.Data, len(*m.session.multiCmd))}
	for k, v := range m.serverCmds {
		var d *resp.Data
		d, err = m.execServer(k)
		if err == nil {
			for index, cmd := range v {
				i := slices.Index(*m.session.multiCmd, cmd)
				if i >= 0 {
					data.Array[i] = d.Array[index]
				} else {
					err = fmt.Errorf("EXECABORT Transaction discarded")
				}
			}
		}
	}
	return data, err
}
