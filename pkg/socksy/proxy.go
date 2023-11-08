package socksy

import (
	"context"
	"fmt"
	"github.com/phayes/freeport"
	"github.com/pkg/errors"
	"github.com/smecsia/welder/pkg/util"
	"inet.af/tcpproxy"
	"net"
	"strings"
	"sync"
)

// SocketProxy represents the socket
type SocketProxy struct {
	network     string
	address     string
	occupyMutex sync.Mutex
}

// NewTcpProxy return a socket using the proxyToPath
func NewTcpProxy(address string) SocketProxy {
	return SocketProxy{
		network:     "tcp",
		address:     address,
		occupyMutex: sync.Mutex{},
	}
}

// NewUnixSocketProxy return a socket using the proxyToPath
func NewUnixSocketProxy(path string) SocketProxy {
	return SocketProxy{
		network:     "unix",
		address:     path,
		occupyMutex: sync.Mutex{},
	}
}

// Start starts proxy and returns its address
func (us *SocketProxy) Start(bindPort int, logger util.Logger) (int, error) {
	var p tcpproxy.Proxy
	if bindPort == 0 {
		port, err := us.randomPort()
		if err != nil {
			return 0, errors.Wrapf(err, "failed to find free port")
		}
		bindPort = port
	}
	addr := fmt.Sprintf("0.0.0.0:%d", bindPort)
	p.AddRoute(addr, &tcpproxy.DialProxy{
		DialContext: func(ctx context.Context, network, address string) (conn net.Conn, e error) {
			return net.Dial(us.network, us.address)
		},
	})
	return bindPort, p.Start()
}

// RandomPort returns the random free open port of the host OS
func (us *SocketProxy) randomPort() (int, error) {
	us.occupyMutex.Lock()
	defer us.occupyMutex.Unlock()
	return freeport.GetFreePort()
}

type ExtrnalIp struct {
	IP        string
	Interface string
}

// GetExternalIPs returns list of external IPs of current host
func GetExternalIPs() ([]ExtrnalIp, error) {
	var ips []ExtrnalIp
	if ifaces, err := net.Interfaces(); err != nil {
		return ips, err
	} else {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 {
				continue // interface down
			} else if iface.Flags&net.FlagLoopback != 0 {
				continue // loopback interface
			} else if strings.HasPrefix(iface.Name, "docker") {
				continue // docker interface
			} else if addrs, err := iface.Addrs(); err != nil {
				return ips, err
			} else {
				for _, addr := range addrs {
					var ip net.IP
					switch v := addr.(type) {
					case *net.IPNet:
						ip = v.IP
					case *net.IPAddr:
						ip = v.IP
					}
					if ip == nil || ip.IsLoopback() || ip.To4() == nil {
						continue
					}
					ips = append(ips, ExtrnalIp{ip.To4().String(), iface.Name})
				}
			}
		}
	}
	if len(ips) > 0 {
		return ips, nil
	} else {
		return ips, errors.New("no external network interfaces found")
	}
}

// GetExternalIPOf returns IP address of network interface
func GetExternalIPOf(extNetwork string) (*ExtrnalIp, error) {
	if ips, err := GetExternalIPs(); err != nil {
		return nil, err
	} else {
		for _, ip := range ips {
			if ip.Interface == extNetwork {
				return &ip, nil
			}
		}
	}
	return nil, errors.New("no such network interface found: " + extNetwork)
}
