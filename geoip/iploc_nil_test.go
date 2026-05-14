package geoip

import (
	"net"
	"testing"
)

func TestLookupNetIPWhenDefaultGeoNil(t *testing.T) {
	orig := defaultGeo
	defaultGeo = nil
	defer func() { defaultGeo = orig }()

	record := LookupNetIP(net.ParseIP("8.8.8.8"))
	if record.Found {
		t.Fatal("expected lookup to fail closed when default geo is nil")
	}
}
