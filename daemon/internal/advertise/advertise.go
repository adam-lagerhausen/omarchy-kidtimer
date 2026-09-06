package advertise

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/grandcat/zeroconf"

	"kidtimer/daemon/internal/netaddr"
)

const ServiceType = "_kidtimer._tcp"
const ServiceLegacy = "_allowance._tcp"
const ServiceParent = "_kidtimer-parent._tcp"

func KidServices() []string {
	return []string{ServiceType, ServiceLegacy}
}

type Found struct {
	ID   string
	Name string
	URL  string
}

func MachineID(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err == nil {
		id := strings.TrimSpace(string(b))
		if id != "" {
			return id, nil
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	id := hex.EncodeToString(raw)
	if err := os.WriteFile(path, []byte(id+"\n"), 0o644); err != nil {
		return "", err
	}
	return id, nil
}

func ParseTXT(txt []string) (id, name string) {
	for _, row := range txt {
		k, v, ok := strings.Cut(row, "=")
		if !ok {
			continue
		}
		switch k {
		case "id":
			id = v
		case "name":
			name = v
		}
	}
	return id, name
}

func KidURL(ips []net.IP, port int) string {
	host := HouseholdHostPort(ips, port, true)
	if host == "" {
		return ""
	}
	return "http://" + host
}

func HouseholdHostPort(ips []net.IP, port int, skipLoopback bool) string {
	var best string
	bestRank := 99
	for _, ip := range ips {
		if ip == nil || ip.IsUnspecified() {
			continue
		}
		c := netaddr.Classify(ip)
		if skipLoopback && c == netaddr.ClassLoopback {
			continue
		}
		rank := householdRank(c)
		if rank < 0 {
			continue
		}
		if rank < bestRank {
			bestRank = rank
			best = net.JoinHostPort(ip.String(), strconv.Itoa(port))
		}
	}
	return best
}

func householdRank(c netaddr.Class) int {
	switch c {
	case netaddr.ClassCGNAT:
		return 0
	case netaddr.ClassPrivate:
		return 1
	case netaddr.ClassLinkLocal:
		return 2
	case netaddr.ClassLoopback:
		return 3
	default:
		return -1
	}
}

func URLHousehold(raw string) bool {
	return netaddr.IsHousehold(HostIP(raw))
}

func HostIP(raw string) net.IP {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "https://")
	if s == "" {
		return nil
	}
	host, _, err := net.SplitHostPort(s)
	if err != nil {
		host = strings.TrimSuffix(s, "/")
		if i := strings.IndexByte(host, '/'); i >= 0 {
			host = host[:i]
		}
	}
	host = strings.Trim(host, "[]")
	if i := strings.IndexByte(host, '%'); i >= 0 {
		host = host[:i]
	}
	return net.ParseIP(host)
}

type Closer interface {
	Shutdown()
}

func Register(name, id string, port int) (Closer, error) {
	return RegisterService(name, ServiceType, id, port)
}

func RegisterService(name, service, id string, port int) (Closer, error) {
	if name == "" {
		name = "kidtimer"
	}
	ifaces := HouseholdIfaces()
	s, err := zeroconf.Register(name, service, "local.", port, []string{"id=" + id, "name=" + name}, ifaces)
	if err != nil && len(ifaces) > 0 {
		s, err = zeroconf.Register(name, service, "local.", port, []string{"id=" + id, "name=" + name}, nil)
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

func Browse(ctx context.Context, out chan<- Found) error {
	return browseKids(ctx, out, BrowseService)
}

func browseKids(ctx context.Context, out chan<- Found, one func(context.Context, string, chan<- Found) error) error {
	if one == nil {
		one = BrowseService
	}
	var wg sync.WaitGroup
	errCh := make(chan error, len(KidServices()))
	for _, svc := range KidServices() {
		wg.Add(1)
		go func(svc string) {
			defer wg.Done()
			if err := one(ctx, svc, out); err != nil {
				errCh <- err
			}
		}(svc)
	}
	wg.Wait()
	close(errCh)
	return <-errCh
}

func BrowseParent(ctx context.Context, out chan<- Found) error {
	return BrowseService(ctx, ServiceParent, out)
}

func BrowseService(ctx context.Context, service string, out chan<- Found) error {
	resolver, err := newResolver()
	if err != nil {
		return err
	}
	entries := make(chan *zeroconf.ServiceEntry)
	go func() {
		for e := range entries {
			id, name := ParseTXT(e.Text)
			if id == "" {
				continue
			}
			if name == "" {
				name = e.Instance
			}
			ips := append([]net.IP{}, e.AddrIPv4...)
			ips = append(ips, e.AddrIPv6...)
			host := HouseholdHostPort(ips, e.Port, false)
			if host == "" {
				continue
			}
			url := "http://" + host
			if service == ServiceType || service == ServiceLegacy {
				url = KidURL(ips, e.Port)
				if url == "" {
					continue
				}
			}
			select {
			case out <- Found{ID: id, Name: name, URL: url}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return resolver.Browse(ctx, service, "local.", entries)
}

func PortOf(addr string) (int, error) {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		return 0, fmt.Errorf("port: %w", err)
	}
	return n, nil
}
