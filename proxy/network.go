package proxy

import (
	"net"
	"sync"

	"github.com/golang/glog"
)

var localIP string
var localIPLock sync.Mutex

func LocalIP() string {
	if len(localIP) != 0 {
		return localIP
	}
	localIPLock.Lock()
	defer localIPLock.Unlock()
	if len(localIP) != 0 {
		return localIP
	}
	localIP = getLocalIP()
	return localIP
}

func getLocalIP() string {
	var result string
	interfaceName := "eth0"
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		glog.Error("get net interface err=%v", err)
		return result
	}

	if iface.Flags&net.FlagUp == 0 {
		glog.Error("net interface %s is down", interfaceName)
		return result
	}

	addrs, err := iface.Addrs()
	if err != nil {
		glog.Error("get addrs of interface %s failed err=%v", interfaceName, err)
		return result
	}

	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip == nil {
			continue
		}
		ip = ip.To4()
		if ip != nil {
			result = ip.String()
			glog.Infof("get local ip %s", result)
			return result
		}
	}
	//
	glog.Error("Failed to get local ip")
	return result
}
