package geoip

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCountryReturnsCountryCode(t *testing.T) {
	orig := defaultGeo
	defer func() { defaultGeo = orig }()

	g := New()
	cc := g.stringTable.GetIndex("US")
	country := g.stringTable.GetIndex("United States")
	record := &TrieRecord{CountryCode: cc, Country: country}

	root := &TrieNode{}
	insertTrie(root, net.ParseIP("8.8.8.0").To4(), 24, record)
	g.trieV4 = root
	g.trieV6 = &TrieNode{}
	g.trieV6 = &TrieNode{}
	defaultGeo = g

	got := Country("8.8.8.8")
	if got != "US" {
		t.Fatalf("expected country code US, got %q", got)
	}
}

func TestRangeToPrefixesCoversNonCIDRRange(t *testing.T) {
	g := New()
	cc := g.stringTable.GetIndex("US")
	record := &TrieRecord{CountryCode: cc}
	root := &TrieNode{}

	prefixes, ok := rangeToPrefixes(net.ParseIP("8.8.8.1"), net.ParseIP("8.8.8.6"))
	if !ok {
		t.Fatal("expected range to be converted")
	}
	if len(prefixes) <= 1 {
		t.Fatalf("expected non-CIDR range to expand into multiple prefixes, got %d", len(prefixes))
	}
	for _, prefix := range prefixes {
		insertTrie(root, prefix.IP.To4(), prefix.PrefixLen, record)
	}
	g.trieV4 = root
	g.trieV6 = &TrieNode{}

	if _, _, _, _, _, _, ok := g.Lookup(net.ParseIP("8.8.8.1")); !ok {
		t.Fatal("expected first IP in range to resolve")
	}
	if _, _, _, _, _, _, ok := g.Lookup(net.ParseIP("8.8.8.6")); !ok {
		t.Fatal("expected last IP in range to resolve")
	}
	if _, _, _, _, _, _, ok := g.Lookup(net.ParseIP("8.8.8.7")); ok {
		t.Fatal("expected IP outside range not to resolve")
	}
}

func TestInitWithErrorUsesExistingCacheWhenCurrent(t *testing.T) {
	origBase := basePath
	origGeo := defaultGeo
	defer func() {
		basePath = origBase
		defaultGeo = origGeo
	}()

	tmp := t.TempDir()
	basePath = tmp

	g := New()
	cc := g.stringTable.GetIndex("US")
	country := g.stringTable.GetIndex("United States")
	record := &TrieRecord{CountryCode: cc, Country: country}
	root := &TrieNode{}
	insertTrie(root, net.ParseIP("8.8.8.0").To4(), 24, record)
	g.trieV4 = root
	g.trieV6 = &TrieNode{}

	if err := g.SaveCache(filepath.Join(tmp, "ipgeo-cache-latest.bin")); err != nil {
		t.Fatalf("save cache: %v", err)
	}
	currentYM := time.Now().Format("2006-01")
	if err := os.WriteFile(filepath.Join(tmp, "latest-ipdb.txt"), []byte(currentYM+"\n"), 0644); err != nil {
		t.Fatalf("write latest marker: %v", err)
	}

	if err := InitWithError(); err != nil {
		t.Fatalf("expected init to load existing cache, got error: %v", err)
	}

	got := Country("8.8.8.8")
	if got != "US" {
		t.Fatalf("expected cache-backed lookup to still work, got %q", got)
	}
}
