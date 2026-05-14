package geoip

import (
	"errors"
	"net"
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
	possibleHeaders = []string{
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

type HeaderOptions struct {
	TrustProxy bool
}

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

func parseIPToken(address string) string {
	token := strings.TrimSpace(address)
	if token == "" {
		return ""
	}

	lower := strings.ToLower(token)
	if idx := strings.Index(lower, "for="); idx >= 0 {
		token = token[idx+4:]
		if sep := strings.Index(token, ";"); sep >= 0 {
			token = token[:sep]
		}
		token = strings.TrimSpace(token)
	}

	token = strings.Trim(token, `"'`)

	// RFC 7239 may wrap IPv6 in square brackets.
	if strings.HasPrefix(token, "[") {
		if idx := strings.Index(token, "]"); idx > 1 {
			candidate := token[1:idx]
			if ip := net.ParseIP(candidate); ip != nil {
				return ip.String()
			}
		}
	}

	if host, _, err := net.SplitHostPort(token); err == nil {
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
	}

	// Handle non-bracketed IPv4 with port.
	if strings.Count(token, ":") == 1 && strings.Contains(token, ".") {
		parts := strings.SplitN(token, ":", 2)
		if ip := net.ParseIP(parts[0]); ip != nil {
			return ip.String()
		}
	}

	if ip := net.ParseIP(token); ip != nil {
		return ip.String()
	}

	return ""
}

// FromHeader retrieves the header value using the provided callback,
// then extracts and returns the first public IP address found.
// If none of the addresses are public, it returns the first IP found in the header.
func FromHeader(clientIP string, callback func(string) string) string {
	return FromHeaderWithOptions(clientIP, callback, HeaderOptions{TrustProxy: true})
}

func FromHeaderWithOptions(clientIP string, callback func(string) string, opts HeaderOptions) string {
	fallback := parseIPToken(clientIP)
	if fallback == "" {
		fallback = "0.0.0.0"
	}
	if !opts.TrustProxy {
		return fallback
	}
	if callback == nil {
		return fallback
	}
	for _, headerName := range possibleHeaders {
		possibleValue := callback(headerName)
		possibleValue = strings.TrimSpace(possibleValue)
		if len(possibleValue) > 3 {
			// Check list of IP in X-Forwarded-For and return the first global address
			for _, address := range strings.Split(possibleValue, ",") {
				ip := parseIPToken(address)
				isPrivate, err := isPrivateAddress(ip)
				if !isPrivate && err == nil {
					return ip
				}
			}
			for _, address := range strings.Split(possibleValue, ",") {
				ip := parseIPToken(address)
				if ip != "" {
					return ip
				}
			}
		}
	}
	return fallback
}

// FromRequest determine user ip
func FromRequest(c ctx.Context) string {
	return FromRequestWithOptions(c, HeaderOptions{TrustProxy: true})
}

func FromRequestWithOptions(c ctx.Context, opts HeaderOptions) string {
	return FromHeaderWithOptions(c.IP(), func(name string) string {
		return c.Get(name)
	}, opts)
}
