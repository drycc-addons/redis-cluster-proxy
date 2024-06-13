package proxy

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"

	resp "github.com/drycc-addons/redis-cluster-proxy/proto"
	"github.com/golang/glog"
)

var (
	OK_DATA *resp.Data
)

func init() {
	OK_DATA = &resp.Data{
		T:      resp.T_SimpleString,
		String: []byte("OK"),
	}
}

/*
multi key cmd被拆分成numKeys个子请求按普通的pipeline request发送，最后在写出response时进行合并
当最后一个子请求的response到来时，整个multi key cmd完成，拼接最终response并写出

只要有一个子请求失败，都认定整个请求失败
多个子请求共享一个request sequence number

请求的失败包含两种类型：1、网络失败，比如读取超时，2，请求错误，比如本来该在A机器上，请求到了B机器上，表现为response type为error
*/
type MultiCmd struct {
	cmd               *resp.Command
	session           *Session
	numSubCmds        int
	numPendingSubCmds int
	subCmdRsps        []*PipelineResponse
}

func NewMultiCmd(session *Session, cmd *resp.Command, numSubCmds int) *MultiCmd {
	mc := &MultiCmd{
		cmd:               cmd,
		session:           session,
		numSubCmds:        numSubCmds,
		numPendingSubCmds: numSubCmds,
	}
	if multiKey, _ := IsMultiCmd(cmd); !multiKey {
		panic("not multi key command")
	}
	mc.subCmdRsps = make([]*PipelineResponse, numSubCmds)
	return mc
}

func (mc *MultiCmd) OnSubCmdFinished(rsp *PipelineResponse) {
	mc.subCmdRsps[rsp.ctx.subSeq] = rsp
	mc.numPendingSubCmds--
}

func (mc *MultiCmd) Finished() bool {
	return mc.numPendingSubCmds == 0
}

func (mc *MultiCmd) CoalesceRsp() *PipelineResponse {
	plRsp := &PipelineResponse{}
	var rsp *resp.Data
	switch getMultiCmdType(mc.cmd) {
	case "SCAN", "READALL", "MGET":
		rsp = &resp.Data{T: resp.T_Array}
	case "MSET":
		rsp = OK_DATA
	case "DEL":
		rsp = &resp.Data{T: resp.T_Integer}
	default:
		panic("invalid multi key cmd name")
	}
	for index, subCmdRsp := range mc.subCmdRsps {
		if subCmdRsp.err != nil {
			rsp = &resp.Data{T: resp.T_Error, String: []byte(subCmdRsp.err.Error())}
			break
		}
		reader := bufio.NewReader(bytes.NewReader(subCmdRsp.rsp.Raw()))
		data, err := resp.ReadData(reader)
		if err != nil {
			glog.Errorf("re-parse response err=%s", err)
			rsp = &resp.Data{T: resp.T_Error, String: []byte(err.Error())}
			break
		}
		if data.T == resp.T_Error {
			rsp = data
			break
		}
		switch getMultiCmdType(mc.cmd) {
		case "READALL":
			if data.Array != nil {
				rsp.Array = append(rsp.Array, data.Array...)
			}
		case "SCAN":
			var key string
			if index == 0 {
				delete(mc.session.cached, fmt.Sprintf("scan:cursor:%s", mc.cmd.Value(1))) // delete old key
				rsp.Array = append(rsp.Array, &resp.Data{T: resp.T_BulkString, String: data.Array[0].String})
				rsp.Array = append(rsp.Array, &resp.Data{T: resp.T_Array})
				key = fmt.Sprintf("scan:cursor:%s", string(data.Array[0].String))
				mc.session.cached[key] = make(map[string]string)
			} else {
				key = fmt.Sprintf("scan:cursor:%s", string(rsp.Array[0].String))
			}
			subKey := fmt.Sprintf("%d", subCmdRsp.ctx.subSeq)
			mc.session.cached[key][subKey] = string(data.Array[0].String)
			rsp.Array[1].Array = append(rsp.Array[1].Array, data.Array[1].Array...)
		case "MGET":
			rsp.Array = append(rsp.Array, data)
		case "MSET", "DEL":
			rsp.Integer += data.Integer
		default:
			panic("invalid multi key cmd name")
		}
	}
	plRsp.rsp = resp.NewObjectFromData(rsp)
	return plRsp
}

func (mc *MultiCmd) SubCmd(index int) (*resp.Command, error) {
	switch getMultiCmdType(mc.cmd) {
	case "MGET":
		return resp.NewCommand("GET", mc.cmd.Value(index+1))
	case "MSET":
		return resp.NewCommand("SET", mc.cmd.Value(2*index+1), mc.cmd.Value((2*index + 2)))
	case "DEL":
		return resp.NewCommand("DEL", mc.cmd.Value(index+1))
	case "SCAN":
		var err error
		var cursor int64
		key := fmt.Sprintf("scan:cursor:%s", mc.cmd.Value(1))
		subKey := fmt.Sprintf("%d", index)
		if data, ok := mc.session.cached[key]; ok {
			if data, ok := data[subKey]; ok {
				cursor, err = strconv.ParseInt(data, 10, 64)
				if err != nil {
					return nil, err
				}
			}
		}
		return resp.NewCommand("SCAN", fmt.Sprintf("%d", cursor))
	default:
		return mc.cmd, nil
	}
}

func IsMultiCmd(cmd *resp.Command) (multiKey bool, numKeys int) {
	multiKey = true
	switch getMultiCmdType(cmd) {
	case "READALL", "MGET", "SCAN":
		numKeys = len(cmd.Args) - 1
	case "MSET":
		numKeys = (len(cmd.Args) - 1) / 2
	case "DEL":
		numKeys = len(cmd.Args) - 1
	default:
		multiKey = false
	}
	return
}

func getMultiCmdType(cmd *resp.Command) string {
	switch cmd.Name() {
	case "MGET", "MSET", "DEL", "SCAN":
		return cmd.Name()
	default:
		if CmdReadAll(cmd) {
			return "READALL"
		}
		return cmd.Name()
	}
}
