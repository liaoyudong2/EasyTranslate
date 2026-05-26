package netutil

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

func LANAddresses(port int) []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return []string{fmt.Sprintf("http://127.0.0.1:%d", port)}
	}

	seen := map[string]struct{}{}
	var candidates []addressCandidate
	for _, iface := range ifaces {
		if !usableInterface(iface) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP.To4()
			if !usableIPv4(ip, ipNet) {
				continue
			}
			url := fmt.Sprintf("http://%s:%d", ip.String(), port)
			if _, exists := seen[url]; exists {
				continue
			}
			seen[url] = struct{}{}
			candidates = append(candidates, addressCandidate{
				URL:      url,
				Priority: interfacePriority(iface.Name),
			})
		}
	}

	if len(candidates) == 0 {
		return []string{fmt.Sprintf("http://127.0.0.1:%d", port)}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Priority == candidates[j].Priority {
			return candidates[i].URL < candidates[j].URL
		}
		return candidates[i].Priority < candidates[j].Priority
	})
	bestPriority := candidates[0].Priority
	var urls []string
	for _, candidate := range candidates {
		if candidate.Priority != bestPriority {
			continue
		}
		urls = append(urls, candidate.URL)
	}
	return urls
}

type addressCandidate struct {
	URL      string
	Priority int
}

func usableInterface(iface net.Interface) bool {
	if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagPointToPoint != 0 {
		return false
	}
	name := strings.ToLower(iface.Name)
	blockedPrefixes := []string{
		"bridge", "vmenet", "vmnet", "utun", "awdl", "llw", "lo", "gif", "stf",
		"docker", "br-", "veth", "tailscale", "zt",
	}
	for _, prefix := range blockedPrefixes {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}
	return true
}

func usableIPv4(ip net.IP, ipNet *net.IPNet) bool {
	if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() {
		return false
	}
	if isNetworkAddress(ip, ipNet) || isBroadcastAddress(ip, ipNet) {
		return false
	}
	return ip.IsPrivate()
}

func interfacePriority(name string) int {
	name = strings.ToLower(name)
	switch {
	case name == "en0":
		return 10
	case strings.HasPrefix(name, "en"):
		return 20
	case strings.HasPrefix(name, "eth"):
		return 30
	case strings.HasPrefix(name, "wlan"), strings.HasPrefix(name, "wi"):
		return 40
	default:
		return 100
	}
}

func isNetworkAddress(ip net.IP, ipNet *net.IPNet) bool {
	network := ip.Mask(ipNet.Mask)
	return network.Equal(ip)
}

func isBroadcastAddress(ip net.IP, ipNet *net.IPNet) bool {
	mask := ipNet.Mask
	if len(mask) == net.IPv6len {
		mask = mask[12:]
	}
	if len(mask) != net.IPv4len {
		return false
	}
	broadcast := make(net.IP, net.IPv4len)
	network := ip.Mask(mask).To4()
	for i := range broadcast {
		broadcast[i] = network[i] | ^mask[i]
	}
	return broadcast.Equal(ip)
}
