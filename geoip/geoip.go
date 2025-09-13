package geoip

import (
	"errors"
	"net"
	"regexp"
	"strings"

	"github.com/oarkflow/ip/ctx"
)

var cidrs []*net.IPNet

func init() {
	maxCidrBlocks := []string{
		"127.0.0.1/8",    // localhost
		"10.0.0.0/8",     // 24-bit block
		"172.16.0.0/12",  // 20-bit block
		"192.168.0.0/16", // 16-bit block
		"169.254.0.0/16", // link local address
		"::1/128",        // localhost IPv6
		"fc00::/7",       // unique local address IPv6
		"fe80::/10",      // link local address IPv6
	}

	cidrs = make([]*net.IPNet, len(maxCidrBlocks))
	for i, maxCidrBlock := range maxCidrBlocks {
		_, cidr, _ := net.ParseCIDR(maxCidrBlock)
		cidrs[i] = cidr
	}
}

var (
	fetchIPFromString = regexp.MustCompile(`(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})`)
	possibleHeaders   = []string{
		"X-Original-Forwarded-For",
		"X-Forwarded-For",
		"X-Real-Ip",
		"X-Real-IP",
		"X-Client-IP",
		"X-Client-Ip",
		"Forwarded-For",
		"Forwarded",
		"Remote-Addr",
		"Client-Ip",
		"CF-Connecting-IP",
		"True-Client-IP",
		"X-Forwarded",
	}
)

func isPrivateAddress(address string) (bool, error) {
	ipAddress := net.ParseIP(address)
	if ipAddress == nil {
		return false, errors.New("address is not valid")
	}
	if ipAddress.IsLoopback() || ipAddress.IsLinkLocalUnicast() || ipAddress.IsLinkLocalMulticast() {
		return true, nil
	}

	for i := range cidrs {
		if cidrs[i].Contains(ipAddress) {
			return true, nil
		}
	}

	return false, nil
}

// FromHeader retrieves the header value using the provided callback,
// then extracts and returns the first public IP address found.
// If none of the addresses are public, it returns the first IP found in the header.
func FromHeader(clientIP string, callback func(string) string) string {
	if callback == nil {
		return clientIP
	}
	var headerValue []byte
	for _, headerName := range possibleHeaders {
		possibleValue := callback(headerName)
		possibleValue = strings.TrimSpace(possibleValue)
		if len([]byte(possibleValue)) > 3 {
			// Check list of IP in X-Forwarded-For and return the first global address
			for _, address := range strings.Split(possibleValue, ",") {
				address = strings.TrimSpace(address)
				isPrivate, err := isPrivateAddress(address)
				if !isPrivate && err == nil {
					return string(fetchIPFromString.Find([]byte(address)))
				}
			}
			return string(fetchIPFromString.Find(headerValue))
		}
	}
	headerValue = []byte(clientIP)
	if len(headerValue) <= 3 {
		headerValue = []byte("0.0.0.0")
	}
	return string(fetchIPFromString.Find(headerValue))
}

// FromRequest determine user ip
func FromRequest(c ctx.Context) string {
	return FromHeader(c.ClientIP(), func(name string) string {
		return string(c.GetHeader(name))
	})
}
