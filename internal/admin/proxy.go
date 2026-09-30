package admin

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ParseTrustedProxies requires explicit proxy IPs or CIDRs. No proxy is trusted by default.
func ParseTrustedProxies(value string) ([]netip.Prefix, error) {
	var result []netip.Prefix
	if strings.TrimSpace(value) == "" {
		return result, nil
	}
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if addr, err := netip.ParseAddr(item); err == nil && addr.Zone() == "" {
			addr = addr.Unmap()
			result = append(result, netip.PrefixFrom(addr, addr.BitLen()))
			continue
		}
		prefix, err := netip.ParsePrefix(item)
		if err != nil {
			return nil, fmt.Errorf("trusted-proxies contains an invalid IP/CIDR: %q", item)
		}
		if prefix.Bits() == 0 {
			return nil, fmt.Errorf("trusted-proxies cannot trust the entire Internet")
		}
		result = append(result, prefix.Masked())
	}
	return result, nil
}
func (h *handler) trusted(addr netip.Addr) bool {
	for _, prefix := range h.proxies {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
func (h *handler) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return "unknown"
	}
	peer = peer.WithZone("").Unmap()
	if !h.trusted(peer) {
		return peer.String()
	}
	forwarded := strings.Join(r.Header.Values("X-Forwarded-For"), ",")
	if forwarded == "" || len(forwarded) > 4096 {
		return peer.String()
	}
	chain := strings.Split(forwarded, ",")
	if len(chain) > 32 {
		return peer.String()
	}
	current := peer
	// Walk from the socket peer towards the client; ignore spoofed left-hand entries.
	for i := len(chain) - 1; i >= 0; i-- {
		if !h.trusted(current) {
			return current.String()
		}
		addr, err := netip.ParseAddr(strings.TrimSpace(chain[i]))
		if err != nil || addr.Zone() != "" || addr.IsUnspecified() {
			return peer.String()
		}
		current = addr.Unmap()
	}
	return current.String()
}
