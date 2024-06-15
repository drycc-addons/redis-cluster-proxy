package proxy

import (
	resp "github.com/drycc-addons/redis-cluster-proxy/proto"
)

const (
	CMD_FLAG_READ = iota
	CMD_FLAG_READ_ALL
	CMD_FLAG_PROXY
	CMD_FLAG_UNKNOWN
	CMD_FLAG_GENERAL
)

/*
*
CMD_FLAG_READ stands for read command
CMD_FLAG_READ_ALL stands for read all command
CMD_FLAG_PROXY stands for proxy command
CMD_FLAG_UNKNOWN stands for unknown command
CMD_FLAG_GENERAL stands for general command
*/
var cmdTable = map[string]int{
	"HELLO":            CMD_FLAG_UNKNOWN,
	"ASKING":           CMD_FLAG_UNKNOWN,
	"AUTH":             CMD_FLAG_PROXY,
	"BGREWRITEAOF":     CMD_FLAG_UNKNOWN,
	"BGSAVE":           CMD_FLAG_UNKNOWN,
	"BITCOUNT":         CMD_FLAG_READ,
	"BITOP":            CMD_FLAG_UNKNOWN,
	"BITPOS":           CMD_FLAG_READ,
	"BLPOP":            CMD_FLAG_UNKNOWN,
	"BRPOP":            CMD_FLAG_UNKNOWN,
	"BRPOPLPUSH":       CMD_FLAG_UNKNOWN,
	"CLIENT":           CMD_FLAG_UNKNOWN,
	"CLUSTER":          CMD_FLAG_UNKNOWN,
	"COMMAND":          CMD_FLAG_READ,
	"CONFIG":           CMD_FLAG_UNKNOWN,
	"DBSIZE":           CMD_FLAG_UNKNOWN,
	"DEBUG":            CMD_FLAG_UNKNOWN,
	"DISCARD":          CMD_FLAG_UNKNOWN,
	"DUMP":             CMD_FLAG_READ,
	"ECHO":             CMD_FLAG_UNKNOWN,
	"EXEC":             CMD_FLAG_READ_ALL,
	"EXISTS":           CMD_FLAG_READ,
	"FLUSHALL":         CMD_FLAG_UNKNOWN,
	"FLUSHDB":          CMD_FLAG_UNKNOWN,
	"GET":              CMD_FLAG_READ,
	"GETBIT":           CMD_FLAG_READ,
	"GETRANGE":         CMD_FLAG_READ,
	"HEXISTS":          CMD_FLAG_READ,
	"HGET":             CMD_FLAG_READ,
	"HGETALL":          CMD_FLAG_READ,
	"HKEYS":            CMD_FLAG_READ,
	"HLEN":             CMD_FLAG_READ,
	"HMGET":            CMD_FLAG_READ,
	"HSCAN":            CMD_FLAG_READ,
	"HVALS":            CMD_FLAG_READ,
	"INFO":             CMD_FLAG_READ,
	"KEYS":             CMD_FLAG_READ_ALL,
	"LASTSAVE":         CMD_FLAG_UNKNOWN,
	"LATENCY":          CMD_FLAG_READ,
	"LINDEX":           CMD_FLAG_READ,
	"LLEN":             CMD_FLAG_READ,
	"LRANGE":           CMD_FLAG_READ,
	"MGET":             CMD_FLAG_READ,
	"MIGRATE":          CMD_FLAG_UNKNOWN,
	"MONITOR":          CMD_FLAG_UNKNOWN,
	"MOVE":             CMD_FLAG_UNKNOWN,
	"MSETNX":           CMD_FLAG_UNKNOWN,
	"MULTI":            CMD_FLAG_READ_ALL,
	"OBJECT":           CMD_FLAG_UNKNOWN,
	"PFCOUNT":          CMD_FLAG_READ,
	"PFSELFTEST":       CMD_FLAG_READ,
	"PING":             CMD_FLAG_PROXY,
	"PSUBSCRIBE":       CMD_FLAG_UNKNOWN,
	"PSYNC":            CMD_FLAG_READ,
	"PTTL":             CMD_FLAG_READ,
	"PUBLISH":          CMD_FLAG_UNKNOWN,
	"PUBSUB":           CMD_FLAG_READ,
	"PUNSUBSCRIBE":     CMD_FLAG_UNKNOWN,
	"RANDOMKEY":        CMD_FLAG_UNKNOWN,
	"READONLY":         CMD_FLAG_READ,
	"READWRITE":        CMD_FLAG_READ,
	"RENAME":           CMD_FLAG_UNKNOWN,
	"RENAMENX":         CMD_FLAG_UNKNOWN,
	"REPLCONF":         CMD_FLAG_READ,
	"SAVE":             CMD_FLAG_UNKNOWN,
	"SCAN":             CMD_FLAG_READ_ALL,
	"SCARD":            CMD_FLAG_READ,
	"SCRIPT":           CMD_FLAG_UNKNOWN,
	"SDIFF":            CMD_FLAG_READ,
	"SELECT":           CMD_FLAG_PROXY,
	"SHUTDOWN":         CMD_FLAG_UNKNOWN,
	"SINTER":           CMD_FLAG_READ,
	"SISMEMBER":        CMD_FLAG_READ,
	"SLAVEOF":          CMD_FLAG_UNKNOWN,
	"SLOWLOG":          CMD_FLAG_READ_ALL,
	"SMEMBERS":         CMD_FLAG_READ,
	"SRANDMEMBER":      CMD_FLAG_READ,
	"SSCAN":            CMD_FLAG_READ,
	"STRLEN":           CMD_FLAG_READ,
	"SUBSCRIBE":        CMD_FLAG_UNKNOWN,
	"SUBSTR":           CMD_FLAG_READ,
	"SUNION":           CMD_FLAG_READ,
	"SYNC":             CMD_FLAG_UNKNOWN,
	"TIME":             CMD_FLAG_UNKNOWN,
	"TTL":              CMD_FLAG_READ,
	"TYPE":             CMD_FLAG_READ,
	"UNSUBSCRIBE":      CMD_FLAG_UNKNOWN,
	"UNWATCH":          CMD_FLAG_UNKNOWN,
	"WAIT":             CMD_FLAG_READ,
	"WATCH":            CMD_FLAG_UNKNOWN,
	"ZCARD":            CMD_FLAG_READ,
	"ZCOUNT":           CMD_FLAG_READ,
	"ZLEXCOUNT":        CMD_FLAG_READ,
	"ZRANGE":           CMD_FLAG_READ,
	"ZRANGEBYLEX":      CMD_FLAG_READ,
	"ZRANGEBYSCORE":    CMD_FLAG_READ,
	"ZRANK":            CMD_FLAG_READ,
	"ZREVRANGE":        CMD_FLAG_READ,
	"ZREVRANGEBYLEX":   CMD_FLAG_READ,
	"ZREVRANGEBYSCORE": CMD_FLAG_READ,
	"ZREVRANK":         CMD_FLAG_READ,
	"ZSCAN":            CMD_FLAG_READ,
	"ZSCORE":           CMD_FLAG_READ,
}

func CmdFlag(cmd *resp.Command) int {
	if flag, ok := cmdTable[cmd.Name()]; ok {
		return flag
	}
	return CMD_FLAG_GENERAL
}

func CmdUnknown(cmd *resp.Command) bool {
	switch CmdFlag(cmd) {
	case CMD_FLAG_UNKNOWN:
		return true
	default:
		return false
	}
}

func CmdAuthRequired(cmd *resp.Command) bool {
	switch cmd.Name() {
	case "AUTH", "HELLO":
		return false
	default:
		return true
	}
}

func CmdReadAll(cmd *resp.Command) bool {
	switch CmdFlag(cmd) {
	case CMD_FLAG_READ_ALL:
		return true
	default:
		return false
	}
}

func CmdReadOnly(cmd *resp.Command) bool {
	switch CmdFlag(cmd) {
	case CMD_FLAG_READ, CMD_FLAG_READ_ALL:
		return true
	default:
		return false
	}
}
