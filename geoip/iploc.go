package geoip

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/csv"
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var trieV4, trieV6 *TrieNode
var wg sync.WaitGroup
var errV4, errV6 error

//go:embed data/dbip-city-ipv4.csv.cache.gz
var ipv4EmbeddedCache []byte

//go:embed data/dbip-city-ipv6.csv.cache.gz
var ipv6EmbeddedCache []byte

var (
	baseURL       = "https://cdn.jsdelivr.net/npm/@ip-location-db/dbip-city-7z/dbip-city-ipv%d.csv.gz"
	ipv4CSV       = "data/dbip-city-ipv4.csv"
	ipv6CSV       = "data/dbip-city-ipv6.csv"
	ipv4CacheFile = ipv4CSV + ".cache.gz"
	ipv6CacheFile = ipv6CSV + ".cache.gz"
	ipv4URL       = fmt.Sprintf(baseURL, 4)
	ipv6URL       = fmt.Sprintf(baseURL, 6)
)

func init() {
	start := time.Now()
	wg.Add(2)

	// Load IPv4 trie concurrently.
	go func() {
		defer wg.Done()
		trieV4, errV4 = loadTrie(ipv4CacheFile, ipv4CSV, ipv4URL, ipv4EmbeddedCache)
	}()

	// Load IPv6 trie concurrently.
	go func() {
		defer wg.Done()
		trieV6, errV6 = loadTrie(ipv6CacheFile, ipv6CSV, ipv6URL, ipv6EmbeddedCache)
	}()

	wg.Wait()
	if errV4 != nil {
		log.Fatalf("Error loading IPv4 trie: %v\n", errV4)
	}
	if errV6 != nil {
		log.Fatalf("Error loading IPv6 trie: %v\n", errV6)
	}
	fmt.Println("Total trie load time:", time.Since(start))
}

// loadTrie applies the following logic:
// 1) If a cache exists (and is newer than CSV), load the trie from cache.
// 2) Else if CSV exists, build trie from CSV and cache it.
// 3) Otherwise, download the gzipped CSV, unzip and save CSV, then build trie and cache it.
// If an embedded cache is provided and available, use it as a fallback.
func loadTrie(cacheFile, csvFile, url string, embeddedCache []byte) (*TrieNode, error) {
	if len(embeddedCache) > 0 {
		fmt.Println("Using embedded cache.")
		return loadTrieFromEmbedded(embeddedCache)
	}
	// First try: if cache exists on disk, use it.
	if info, err := os.Stat(cacheFile); err == nil {
		// If CSV exists, ensure cache is newer than CSV.
		if csvInfo, err := os.Stat(csvFile); err == nil && info.ModTime().After(csvInfo.ModTime()) {
			fmt.Printf("Loading trie from cache file: %s\n", cacheFile)
			if trie, err := loadTrieFromCache(cacheFile); err == nil {
				return trie, nil
			}
		} else if csvInfo == nil {
			// If CSV doesn't exist, still try to load from cache.
			fmt.Printf("Loading trie from cache file: %s\n", cacheFile)
			if trie, err := loadTrieFromCache(cacheFile); err == nil {
				return trie, nil
			}
		}
	}

	// Second try: if CSV exists, load trie from CSV then cache it.
	if _, err := os.Stat(csvFile); err == nil {
		fmt.Printf("Building trie from CSV file: %s\n", csvFile)
		trie, err := loadTrieFromCSV(csvFile)
		if err == nil {
			if err := saveTrieToCache(cacheFile, trie); err != nil {
				fmt.Printf("Warning: failed to save cache to %s: %v\n", cacheFile, err)
			}
		}
		return trie, err
	}

	// Third try: Neither CSV nor cache exist.
	fmt.Printf("CSV file not found, downloading from URL: %s\n", url)
	if err := downloadAndUnzipCSV(url, csvFile); err != nil {
		// As a fallback, try using the embedded cache if available.
		if len(embeddedCache) > 0 {
			fmt.Println("Using embedded cache.")
			return loadTrieFromEmbedded(embeddedCache)
		}
		return nil, fmt.Errorf("failed to download CSV: %w", err)
	}

	// Now CSV should exist. Build trie from CSV and cache it.
	trie, err := loadTrieFromCSV(csvFile)
	if err == nil {
		if err := saveTrieToCache(cacheFile, trie); err != nil {
			fmt.Printf("Warning: failed to save cache to %s: %v\n", cacheFile, err)
		}
	}
	return trie, err
}

// GeoRecord holds geolocation information.
type GeoRecord struct {
	Network   string // in CIDR notation
	Country   string
	Region    string
	City      string
	Latitude  float64
	Longitude float64
}

// TrieNode is a node in the IP prefix trie.
// Exported fields for gob serialization.
type TrieNode struct {
	Left   *TrieNode  // bit 0
	Right  *TrieNode  // bit 1
	Record *GeoRecord // non-nil if a record is stored at this prefix
}

// getBit returns the i-th bit of the IP address (i=0 is the most significant bit).
// For IPv4 addresses, it uses the 4-byte representation.
func getBit(ip net.IP, i int) int {
	if ip4 := ip.To4(); ip4 != nil {
		byteIndex := i / 8
		bitIndex := 7 - uint(i%8)
		return int((ip4[byteIndex] >> bitIndex) & 1)
	}
	// Otherwise, use the 16-byte representation for IPv6.
	ip = ip.To16()
	byteIndex := i / 8
	bitIndex := 7 - uint(i%8)
	return int((ip[byteIndex] >> bitIndex) & 1)
}

// insertTrie inserts a GeoRecord into the trie for the given IP starting at ip with prefixLen bits.
func insertTrie(root *TrieNode, ip net.IP, prefixLen int, record *GeoRecord) {
	node := root
	for i := 0; i < prefixLen; i++ {
		if getBit(ip, i) == 0 {
			if node.Left == nil {
				node.Left = &TrieNode{}
			}
			node = node.Left
		} else {
			if node.Right == nil {
				node.Right = &TrieNode{}
			}
			node = node.Right
		}
	}
	node.Record = record
}

// lookupTrie performs a longest prefix match lookup on the trie for the given IP.
func lookupTrie(root *TrieNode, ip net.IP, maxBits int) *GeoRecord {
	var result *GeoRecord
	node := root
	for i := 0; i < maxBits; i++ {
		if node == nil {
			break
		}
		if node.Record != nil {
			result = node.Record
		}
		if getBit(ip, i) == 0 {
			node = node.Left
		} else {
			node = node.Right
		}
	}
	return result
}

// computePrefixLen determines the prefix length for a range defined by start and end IP.
// It verifies that the range corresponds exactly to a CIDR block.
func computePrefixLen(start, end net.IP) (int, error) {
	// Normalize to proper representation.
	if s4 := start.To4(); s4 != nil {
		start = s4
	}
	if e4 := end.To4(); e4 != nil {
		end = e4
	}

	var maxBits int
	if start.To4() != nil {
		maxBits = 32
	} else {
		maxBits = 128
	}

	// Find the first bit where start and end differ.
	prefixLen := 0
	for i := 0; i < maxBits; i++ {
		if getBit(start, i) != getBit(end, i) {
			break
		}
		prefixLen++
	}

	// Verify that start has zeros and end has ones in the remaining bits.
	for i := prefixLen; i < maxBits; i++ {
		if getBit(start, i) != 0 {
			return 0, fmt.Errorf("start IP not aligned for a CIDR block")
		}
		if getBit(end, i) != 1 {
			return 0, fmt.Errorf("end IP not a proper broadcast for a CIDR block")
		}
	}
	return prefixLen, nil
}

// downloadAndUnzipCSV downloads a gzipped CSV from the given URL,
// unzips it, and writes the resulting CSV to the given file path.
func downloadAndUnzipCSV(url, csvFile string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download CSV, status: %s", resp.Status)
	}

	// Create a gzip reader from the response.
	gzReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("error creating gzip reader: %w", err)
	}
	defer gzReader.Close()

	// Create or truncate the target CSV file.
	outFile, err := os.Create(csvFile)
	if err != nil {
		return err
	}
	defer outFile.Close()

	// Copy the uncompressed data to the file.
	_, err = io.Copy(outFile, gzReader)
	return err
}

// saveTrieToCache saves a trie to a cache file using gob with gzip compression.
func saveTrieToCache(cacheFile string, trie *TrieNode) error {
	f, err := os.Create(cacheFile)
	if err != nil {
		return fmt.Errorf("error creating cache file %s: %w", cacheFile, err)
	}
	defer f.Close()

	gzWriter := gzip.NewWriter(f)
	defer gzWriter.Close()

	encoder := gob.NewEncoder(gzWriter)
	return encoder.Encode(trie)
}

// loadTrieFromCache loads a trie from a cache file using gob with gzip decompression.
func loadTrieFromCache(cacheFile string) (*TrieNode, error) {
	f, err := os.Open(cacheFile)
	if err != nil {
		return nil, fmt.Errorf("error opening cache file %s: %w", cacheFile, err)
	}
	defer f.Close()

	gzReader, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("error creating gzip reader for %s: %w", cacheFile, err)
	}
	defer gzReader.Close()

	var trie TrieNode
	decoder := gob.NewDecoder(gzReader)
	if err := decoder.Decode(&trie); err != nil {
		return nil, fmt.Errorf("error decoding cache file %s: %w", cacheFile, err)
	}
	return &trie, nil
}

// loadTrieFromCSV loads a trie from the provided CSV file.
func loadTrieFromCSV(filename string) (*TrieNode, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("error opening CSV file %s: %w", filename, err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("error reading CSV file %s: %w", filename, err)
	}

	root := &TrieNode{}
	for _, row := range records {
		if len(row) < 9 {
			continue
		}

		startIPStr := strings.TrimSpace(row[0])
		endIPStr := strings.TrimSpace(row[1])
		country := strings.TrimSpace(row[2])
		region := strings.TrimSpace(row[3])
		city := strings.TrimSpace(row[5])
		latStr := strings.TrimSpace(row[7])
		lonStr := strings.TrimSpace(row[8])

		startIP := net.ParseIP(startIPStr)
		endIP := net.ParseIP(endIPStr)
		if startIP == nil || endIP == nil {
			continue
		}

		prefixLen, err := computePrefixLen(startIP, endIP)
		if err != nil {
			continue
		}

		lat, err1 := strconv.ParseFloat(latStr, 64)
		lon, err2 := strconv.ParseFloat(lonStr, 64)
		if err1 != nil || err2 != nil {
			continue
		}

		cidr := fmt.Sprintf("%s/%d", startIP.String(), prefixLen)
		geo := &GeoRecord{
			Network:   cidr,
			Country:   country,
			Region:    region,
			City:      city,
			Latitude:  lat,
			Longitude: lon,
		}

		if startIP.To4() != nil {
			insertTrie(root, startIP.To4(), prefixLen, geo)
		} else {
			insertTrie(root, startIP, prefixLen, geo)
		}
	}
	return root, nil
}

// loadTrieFromEmbedded loads a trie from an embedded compressed cache.
func loadTrieFromEmbedded(data []byte) (*TrieNode, error) {
	gzReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("error creating gzip reader: %w", err)
	}
	defer gzReader.Close()

	var trie TrieNode
	decoder := gob.NewDecoder(gzReader)
	if err := decoder.Decode(&trie); err != nil {
		return nil, fmt.Errorf("error decoding embedded trie: %w", err)
	}
	return &trie, nil
}

// LookupGeoIP looks up the geolocation for a given IP (net.IP) using the loaded tries.
func LookupGeoIP(ip net.IP) (*GeoRecord, error) {
	var record *GeoRecord
	if ip.To4() != nil {
		record = lookupTrie(trieV4, ip.To4(), 32)
	} else {
		record = lookupTrie(trieV6, ip, 128)
	}
	if record == nil {
		return nil, fmt.Errorf("no geolocation record found for %s", ip)
	}
	return record, nil
}

// LookupGeo looks up the geolocation for a given IP address string.
func LookupGeo(ipStr string) (*GeoRecord, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP address: %s", ipStr)
	}
	return LookupGeoIP(ip)
}

// countryByIP returns the ISO 3166-1 alpha-2 country code as bytes for a given IP.
func countryByIP(ip net.IP) []byte {
	if ip == nil {
		return nil
	}
	record, err := LookupGeoIP(ip)
	if err != nil {
		fmt.Println("Error during lookup:", err.Error())
		return nil
	}
	return []byte(record.Country)
}

// Country returns the country code as a string for a given IP address string.
func Country(ip string) string {
	return string(countryByIP(net.ParseIP(ip)))
}

// CountryByIP returns the country code as a string for a given net.IP.
func CountryByIP(ip net.IP) string {
	return string(countryByIP(ip))
}

// IsReservedIPv4 detects whether a net.IP is a reserved IPv4 address. Returns false for IPv6.
func IsReservedIPv4(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	switch ip4[0] {
	case 10:
		return true
	case 100:
		return ip4[1] >= 64 && ip4[1] <= 127
	case 127:
		return true
	case 169:
		return ip4[1] == 254
	case 172:
		return ip4[1] >= 16 && ip4[1] <= 31
	case 192:
		switch ip4[1] {
		case 0:
			return ip4[2] == 0 || ip4[2] == 2
		case 18, 19:
			return true
		case 51:
			return ip4[2] == 100
		case 88:
			return ip4[2] == 99
		case 168:
			return true
		}
	case 203:
		return ip4[1] == 0 && ip4[2] == 113
	case 224, 240:
		return true
	}
	return false
}
