package proxy

import (
	"bufio"
	"bytes"

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
	numSubCmds        int
	numPendingSubCmds int
	subCmdRsps        []*PipelineResponse
}

func NewMultiCmd(cmd *resp.Command, numSubCmds int) *MultiCmd {
	mc := &MultiCmd{
		cmd:               cmd,
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
	switch mc.cmd.Name() {
	case "KEYS", "MGET":
		rsp = &resp.Data{T: resp.T_Array}
	case "MSET":
		rsp = OK_DATA
	case "DEL":
		rsp = &resp.Data{T: resp.T_Integer}
	default:
		panic("invalid multi key cmd name")
	}
	for _, subCmdRsp := range mc.subCmdRsps {
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
		switch mc.cmd.Name() {
		case "KEYS":
			if data.Array != nil {
				rsp.Array = append(rsp.Array, data.Array...)
			}
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

func IsMultiCmd(cmd *resp.Command) (multiKey bool, numKeys int) {
	multiKey = true
	switch cmd.Name() {
	case "KEYS", "MGET":
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
