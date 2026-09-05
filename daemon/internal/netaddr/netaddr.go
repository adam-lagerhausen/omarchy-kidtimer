package netaddr

import (
	"fmt"
	"net"
	"strings"
)

const DefaultListen = "127.0.0.1:8742"

type Class int

const (
	ClassPublic Class = iota
	ClassLoopback
	ClassPrivate
	ClassCGNAT
	ClassLinkLocal
)

var cgnat = mustCIDR("100.64.0.0/10")

func mustCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return n
}

func Classify(ip net.IP) Class {
	if ip == nil || ip.IsUnspecified() {
		return ClassPublic
	}
	if ip.IsLoopback() {
		return ClassLoopback
	}
	if ip.IsLinkLocalUnicast() {
		return ClassLinkLocal
	}
	if cgnat.Contains(ip) {
		return ClassCGNAT
	}
	if ip.IsPrivate() {
		return ClassPrivate
	}
	return ClassPublic
}

func IsHousehold(ip net.IP) bool {
	switch Classify(ip) {
	case ClassLoopback, ClassPrivate, ClassCGNAT, ClassLinkLocal:
		return true
	default:
		return false
	}
}

func IsPrivate(ip net.IP) bool {
	return IsHousehold(ip)
}

func PeerIP(remoteAddr string) net.IP {
	host := strings.TrimSpace(remoteAddr)
	if host == "" {
		return nil
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return net.ParseIP(host)
}

func ParseListen(spec string) ([]string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return []string{DefaultListen}, nil
	}
	var out []string
	seen := map[string]bool{}
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, _, err := net.SplitHostPort(part); err != nil {
			return nil, fmt.Errorf("listen: %w", err)
		}
		if seen[part] {
			continue
		}
		seen[part] = true
		out = append(out, part)
	}
	if len(out) == 0 {
		return []string{DefaultListen}, nil
	}
	return out, nil
}

func TryListen(addr string) (net.Listener, error) {
	if zoned := withLinkLocalZone(addr); zoned != "" && zoned != addr {
		ln, err := net.Listen("tcp", zoned)
		if err == nil {
			return ln, nil
		}
	}
	return net.Listen("tcp", addr)
}

func withLinkLocalZone(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return ""
	}
	if strings.Contains(host, "%") {
		return addr
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil || !ip.IsLinkLocalUnicast() {
		return ""
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		addrs, err := ifaceAddrs(iface)
		if err != nil {
			continue
		}
		for _, a := range addrs {
			got := ipOf(a)
			if got != nil && got.Equal(ip) {
				return net.JoinHostPort(ip.String()+"%"+iface.Name, port)
			}
		}
	}
	return ""
}

var ifaceAddrs = func(iface net.Interface) ([]net.Addr, error) {
	return iface.Addrs()
}

func ipOf(a net.Addr) net.IP {
	switch v := a.(type) {
	case *net.IPNet:
		return v.IP
	case *net.IPAddr:
		return v.IP
	default:
		return nil
	}
}
