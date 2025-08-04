package protect

import (
	"errors"
	"fmt"
	"net"
)

var privateIPBlocks []*net.IPNet

func init() {
	for _, cidr := range []string{
		"127.0.0.0/8",    // localhost
		"10.0.0.0/8",     // private network
		"172.16.0.0/12",  // private network
		"192.168.0.0/16", // private network
		"169.254.0.0/16", // link-local
		"::1/128",        // IPv6 loopback
		"fc00::/7",       // IPv6 unique local
		"fe80::/10",      // IPv6 link-local
	} {
		_, block, _ := net.ParseCIDR(cidr)
		privateIPBlocks = append(privateIPBlocks, block)
	}
}

func IsPrivateIP(ip net.IP) bool {
	for _, block := range privateIPBlocks {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}

var ErrSSRF = errors.New("ssrf protection")

func SSRFProtectLookup(host string) (net.IP, error) {
	// Убираем порт
	h, _, _ := net.SplitHostPort(host)
	if h != "" {
		host = h
	}

	// Резолвим DNS
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, errors.New("no IP addresses found")
	}

	for _, ip := range ips {
		if IsPrivateIP(ip) {
			return nil, fmt.Errorf("%w: private IP %s is not allowed", ErrSSRF, ip)
		}
	}

	return ips[0], nil
}
