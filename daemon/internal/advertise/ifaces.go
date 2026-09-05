package advertise

import (
	"net"

	"github.com/grandcat/zeroconf"

	"kidtimer/daemon/internal/netaddr"
)

func HouseholdIfaces() []net.Interface {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []net.Interface
	for _, ifi := range ifaces {
		if !Advertisable(ifi, ifaceIPs(ifi)) {
			continue
		}
		out = append(out, ifi)
	}
	return out
}

func Advertisable(ifi net.Interface, ips []net.IP) bool {
	if ifi.Flags&net.FlagUp == 0 {
		return false
	}
	if ifi.Flags&net.FlagLoopback != 0 {
		return false
	}
	hasHH := false
	for _, ip := range ips {
		if ip == nil || ip.IsLoopback() {
			continue
		}
		if netaddr.IsHousehold(ip) {
			hasHH = true
			break
		}
	}
	if !hasHH {
		return false
	}
	return ifi.Flags&net.FlagMulticast != 0 || ifi.Flags&net.FlagPointToPoint != 0
}

func ifaceIPs(ifi net.Interface) []net.IP {
	addrs, err := ifi.Addrs()
	if err != nil {
		return nil
	}
	var out []net.IP
	for _, a := range addrs {
		switch v := a.(type) {
		case *net.IPNet:
			out = append(out, v.IP)
		case *net.IPAddr:
			out = append(out, v.IP)
		}
	}
	return out
}

func newResolver() (*zeroconf.Resolver, error) {
	ifaces := HouseholdIfaces()
	opts := []zeroconf.ClientOption{zeroconf.SelectIPTraffic(zeroconf.IPv4)}
	if len(ifaces) > 0 {
		opts = append(opts, zeroconf.SelectIfaces(ifaces))
	}
	r, err := zeroconf.NewResolver(opts...)
	if err == nil {
		return r, nil
	}
	return zeroconf.NewResolver(zeroconf.SelectIPTraffic(zeroconf.IPv4))
}
