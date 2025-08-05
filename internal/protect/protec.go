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

// ReplaceHostToIP резолвит хост, проверяет ip, возвращает адрес в котором host заменен на ip.
// Возвращает любые ошибки которые возникаю при разрешении хоста. Если ip локальный, возвращает ошибку ErrSSRF.
func ReplaceHostToIP(host string) (string, error) {
	host, port, _ := net.SplitHostPort(host)

	// Резолвим DNS
	ips, err := net.LookupIP(host)
	if err != nil {
		return "", err
	}
	if len(ips) == 0 {
		return "", errors.New("no IP addresses found")
	}

	for _, ip := range ips {
		if IsPrivateIP(ip) {
			return "", fmt.Errorf("%w: private IP %s is not allowed", ErrSSRF, ip)
		}
	}

	return ips[0].String() + ":" + port, nil
}
