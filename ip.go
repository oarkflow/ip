package ip

import (
	"net"
	"time"

	"github.com/oarkflow/ip/ctx"
	"github.com/oarkflow/ip/geoip"
)

func Init() {
	geoip.Init()
}

// Country is a simple IP-country code lookup.
// Returns an empty string when cannot determine country.
func Country(ip string) string {
	return geoip.Country(ip)
}

func Lookup(ip string) geoip.GeoRecord {
	return geoip.Lookup(ip)
}

func LookupNetIP(ip net.IP) geoip.GeoRecord {
	return geoip.LookupNetIP(ip)
}

// CountryByNetIP is a simple IP-country code lookup.
// Returns an empty string when cannot determine country.
func CountryByNetIP(ip net.IP) string {
	return geoip.CountryByIP(ip)
}

func Detect(c ctx.Context) error {
	ip := FromRequest(c)
	c.Locals("ip", ip)
	c.Locals("ip_country", Country(ip))
	return c.Next()
}

func FromRequest(c ctx.Context) string {
	return geoip.FromRequest(c)
}

func FromHeader(clientIP string, callback func(string) string) string {
	return geoip.FromHeader(clientIP, callback)
}

func ChangeTimezone(dt time.Time, timezone string) time.Time {
	loc, _ := time.LoadLocation(timezone)
	newTime := dt.In(loc)
	return newTime
}
