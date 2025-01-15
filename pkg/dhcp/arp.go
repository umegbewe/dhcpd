package dhcp

import (
	"fmt"
	"net"
	"time"

	"github.com/j-keck/arping"
)

func ARPCheck(iface *net.Interface, ip net.IP, timeout time.Duration) (bool, error) {
	ip4 := ip.To4()

	if ip4 == nil {
		return false, fmt.Errorf("ARP check only valid for IPv4, got: %v", ip)
	}

	_, _, err := arping.PingOverIface(ip4, *iface)
	switch err {
	case nil:
		// `nil` error => got a reply => IP in use
		return true, nil
	case arping.ErrTimeout:
		// timed out => IP not in use
		return false, nil
	default:
		return false, err
	}
}
