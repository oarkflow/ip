package ip

import (
	"context"
	"net"
	"time"

	"github.com/oarkflow/ip/ctx"
	"github.com/oarkflow/ip/geoip"
)

// Country is a simple IP-country code lookup.
// Returns an empty string when cannot determine country.
func Country(ip string) string {
	return geoip.Country(ip)
}

func GeoLocation(ip string) (*geoip.GeoRecord, error) {
	return geoip.LookupGeo(ip)
}

func GeoLocationIP(ip net.IP) (*geoip.GeoRecord, error) {
	return geoip.LookupGeoIP(ip)
}

// CountryByNetIP is a simple IP-country code lookup.
// Returns an empty string when cannot determine country.
func CountryByNetIP(ip net.IP) string {
	return geoip.CountryByIP(ip)
}

func Detect(ctx context.Context, c ctx.Context) {
	ip := FromRequest(c)
	c.Set("ip", ip)
	c.Set("ip_country", Country(ip))
	c.Next(ctx)
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
