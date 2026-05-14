package geoip

import (
	"bufio"
	"compress/gzip"
	"encoding/binary"
	"encoding/csv"
	"encoding/gob"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var basePath = "./.ipdata"

func SetBasePath(path string) {
	basePath = path
}

type StringTable struct {
	strings []string
	lookup  map[string]uint32
	mu      sync.RWMutex
}

func NewStringTable() *StringTable {
	return &StringTable{
		strings: make([]string, 1), // Index 0 = empty string
		lookup:  make(map[string]uint32),
	}
}

func (st *StringTable) GetIndex(s string) uint32 {
	if s == "" {
		return 0
	}

	st.mu.RLock()
	if idx, exists := st.lookup[s]; exists {
		st.mu.RUnlock()
		return idx
	}
	st.mu.RUnlock()

	st.mu.Lock()
	defer st.mu.Unlock()

	// Double-check after acquiring write lock
	if idx, exists := st.lookup[s]; exists {
		return idx
	}

	idx := uint32(len(st.strings))
	st.strings = append(st.strings, s)
	st.lookup[s] = idx
	return idx
}

func (st *StringTable) GetString(idx uint32) string {
	st.mu.RLock()
	defer st.mu.RUnlock()
	if int(idx) >= len(st.strings) {
		return ""
	}
	return st.strings[idx]
}

type TrieRecord struct {
	CountryCode uint32
	Country     uint32
	Region      uint32
	City        uint32
	Lat         float32
	Lng         float32
}

type TrieNode struct {
	Left   *TrieNode
	Right  *TrieNode
	Record *TrieRecord
}

type IPGeo struct {
	trieV4      *TrieNode
	trieV6      *TrieNode
	stringTable *StringTable
	mu          sync.RWMutex
}

const (
	cacheVersion = 3
	cacheHeader  = "IPGEO_CACHE"
)

func New() *IPGeo {
	return &IPGeo{
		stringTable: NewStringTable(),
	}
}

// getBit returns the i-th bit of the IP address (i=0 is the most significant bit).
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

// insertTrie inserts a TrieRecord into the trie for the given IP starting at ip with prefixLen bits.
func insertTrie(root *TrieNode, ip net.IP, prefixLen int, record *TrieRecord) {
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
func lookupTrie(root *TrieNode, ip net.IP, maxBits int) *TrieRecord {
	var result *TrieRecord
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
	if node != nil && node.Record != nil {
		result = node.Record
	}
	return result
}

// computePrefixLen determines the prefix length for a range defined by start and end IP.
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

type ipPrefix struct {
	IP        net.IP
	PrefixLen int
}

func normalizeRange(start, end net.IP) (net.IP, net.IP, int, bool) {
	s4 := start.To4()
	e4 := end.To4()
	if s4 != nil || e4 != nil {
		if s4 == nil || e4 == nil {
			return nil, nil, 0, false
		}
		return s4, e4, 32, true
	}
	s16 := start.To16()
	e16 := end.To16()
	if s16 == nil || e16 == nil {
		return nil, nil, 0, false
	}
	return s16, e16, 128, true
}

func ipToInt(ip net.IP) *big.Int {
	return new(big.Int).SetBytes(ip)
}

func intToIP(n *big.Int, bits int) net.IP {
	size := bits / 8
	out := make([]byte, size)
	raw := n.Bytes()
	copy(out[size-len(raw):], raw)
	return net.IP(out)
}

func rangeToPrefixes(start, end net.IP) ([]ipPrefix, bool) {
	start, end, bits, ok := normalizeRange(start, end)
	if !ok {
		return nil, false
	}
	current := ipToInt(start)
	last := ipToInt(end)
	if current.Cmp(last) > 0 {
		return nil, false
	}

	one := big.NewInt(1)
	prefixes := make([]ipPrefix, 0, 1)
	for current.Cmp(last) <= 0 {
		remaining := new(big.Int).Sub(last, current)
		remaining.Add(remaining, one)

		trailingZeros := bits
		if current.Sign() != 0 {
			trailingZeros = int(current.TrailingZeroBits())
			if trailingZeros > bits {
				trailingZeros = bits
			}
		}

		prefixLen := bits - trailingZeros
		for {
			blockSize := new(big.Int).Lsh(one, uint(bits-prefixLen))
			if blockSize.Cmp(remaining) <= 0 {
				prefixes = append(prefixes, ipPrefix{
					IP:        intToIP(current, bits),
					PrefixLen: prefixLen,
				})
				current.Add(current, blockSize)
				break
			}
			prefixLen++
		}
	}
	return prefixes, true
}

func getDBUrl(year, month string) string {
	return fmt.Sprintf("https://download.db-ip.com/free/dbip-city-lite-%s-%s.csv.gz", year, month)
}

type httpStatusError struct {
	StatusCode int
}

func (e httpStatusError) Error() string {
	return fmt.Sprintf("HTTP error: %d", e.StatusCode)
}

func download(url, filePath string) error {
	fmt.Printf("Downloading %s...\n", url)
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("failed to create download directory %s: %w", filepath.Dir(filePath), err)
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return httpStatusError{StatusCode: resp.StatusCode}
	}

	tmpPath := filePath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	if _, err = io.Copy(out, resp.Body); err != nil {
		out.Close()
		os.Remove(tmpPath)
		return err
	}
	if err = out.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, filePath)
}

func csvPathForAvailableMonth(start time.Time) (string, string, error) {
	for current := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, start.Location()); current.Year() > 0; current = current.AddDate(0, -1, 0) {
		yearStr := fmt.Sprintf("%d", current.Year())
		monthStr := fmt.Sprintf("%02d", int(current.Month()))
		csvPath := getCSVPath(yearStr, monthStr)
		if _, err := os.Stat(csvPath); err == nil {
			return csvPath, fmt.Sprintf("%s-%s", yearStr, monthStr), nil
		} else if !os.IsNotExist(err) {
			return "", "", err
		}

		url := getDBUrl(yearStr, monthStr)
		if err := download(url, csvPath); err != nil {
			if statusErr, ok := err.(httpStatusError); ok && statusErr.StatusCode == http.StatusNotFound {
				fmt.Printf("No DB-IP data for %s-%s, trying previous month...\n", yearStr, monthStr)
				continue
			}
			return "", "", err
		}
		return csvPath, fmt.Sprintf("%s-%s", yearStr, monthStr), nil
	}
	return "", "", fmt.Errorf("could not find downloadable DB-IP data")
}

func (g *IPGeo) SaveCache(cachePath string) error {
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return err
	}
	tmpPath := cachePath + ".tmp"
	file, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	defer file.Close()
	cleanup := true
	defer func() {
		if cleanup {
			os.Remove(tmpPath)
		}
	}()

	writer := bufio.NewWriter(file)

	// Write header and version
	if _, err := writer.WriteString(cacheHeader); err != nil {
		return err
	}
	if err := binary.Write(writer, binary.LittleEndian, uint32(cacheVersion)); err != nil {
		return err
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	// Write string table
	if err := binary.Write(writer, binary.LittleEndian, uint32(len(g.stringTable.strings))); err != nil {
		return err
	}
	for _, s := range g.stringTable.strings {
		if err := binary.Write(writer, binary.LittleEndian, uint32(len(s))); err != nil {
			return err
		}
		if _, err := writer.WriteString(s); err != nil {
			return err
		}
	}

	// Write tries using gob
	encoder := gob.NewEncoder(writer)
	if err := encoder.Encode(g.trieV4); err != nil {
		return err
	}
	if err := encoder.Encode(g.trieV6); err != nil {
		return err
	}

	if err := writer.Flush(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, cachePath); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func (g *IPGeo) LoadCache(cachePath string) error {
	file, err := os.Open(cachePath)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := bufio.NewReader(file)

	// Read and verify header
	headerBuf := make([]byte, len(cacheHeader))
	if _, err := io.ReadFull(reader, headerBuf); err != nil {
		return err
	}
	if string(headerBuf) != cacheHeader {
		return fmt.Errorf("invalid cache file format")
	}

	var version uint32
	if err := binary.Read(reader, binary.LittleEndian, &version); err != nil {
		return err
	}
	if version != cacheVersion {
		return fmt.Errorf("incompatible cache version")
	}

	// Read string table
	var stringCount uint32
	if err := binary.Read(reader, binary.LittleEndian, &stringCount); err != nil {
		return err
	}

	stringTable := NewStringTable()
	stringTable.strings = make([]string, stringCount)
	stringTable.lookup = make(map[string]uint32)

	for i := uint32(0); i < stringCount; i++ {
		var strLen uint32
		if err := binary.Read(reader, binary.LittleEndian, &strLen); err != nil {
			return err
		}
		strBuf := make([]byte, strLen)
		if _, err := io.ReadFull(reader, strBuf); err != nil {
			return err
		}
		str := string(strBuf)
		stringTable.strings[i] = str
		if str != "" {
			stringTable.lookup[str] = i
		}
	}

	// Read tries using gob
	decoder := gob.NewDecoder(reader)
	var trieV4 *TrieNode
	var trieV6 *TrieNode
	if err := decoder.Decode(&trieV4); err != nil {
		return err
	}
	if err := decoder.Decode(&trieV6); err != nil {
		return err
	}

	g.mu.Lock()
	g.stringTable = stringTable
	g.trieV4 = trieV4
	g.trieV6 = trieV6
	g.mu.Unlock()

	fmt.Printf("Loaded cache successfully\n")
	return nil
}

// LoadDBIP loads DB-IP City Lite CSV.GZ file with optimizations
func (g *IPGeo) LoadDBIP(path string) error {
	fmt.Printf("Parsing CSV file: %s\n", path)
	start := time.Now()

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	// Use larger buffer for better I/O performance
	bufferedReader := bufio.NewReaderSize(gz, 64*1024)
	r := csv.NewReader(bufferedReader)
	r.FieldsPerRecord = -1
	r.ReuseRecord = true // Reuse record slice for better memory efficiency

	// Build tries
	rootV4 := &TrieNode{}
	rootV6 := &TrieNode{}
	stringTable := NewStringTable()
	v4Count := 0
	v6Count := 0
	skippedCount := 0

	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		if len(rec) < 8 {
			skippedCount++
			continue
		}

		startIP := strings.TrimSpace(rec[0])
		endIP := strings.TrimSpace(rec[1])
		cc := stringTable.GetIndex(rec[2])
		country := stringTable.GetIndex(rec[3])
		region := stringTable.GetIndex(rec[4])
		city := stringTable.GetIndex(rec[5])

		lat, _ := strconv.ParseFloat(rec[6], 32)
		lng, _ := strconv.ParseFloat(rec[7], 32)

		sip := net.ParseIP(startIP)
		eip := net.ParseIP(endIP)
		if sip == nil || eip == nil {
			skippedCount++
			continue
		}

		prefixes, ok := rangeToPrefixes(sip, eip)
		if !ok {
			skippedCount++
			continue
		}

		record := &TrieRecord{
			CountryCode: cc,
			Country:     country,
			Region:      region,
			City:        city,
			Lat:         float32(lat),
			Lng:         float32(lng),
		}

		for _, prefix := range prefixes {
			if prefix.IP.To4() != nil {
				insertTrie(rootV4, prefix.IP.To4(), prefix.PrefixLen, record)
				v4Count++
			} else {
				insertTrie(rootV6, prefix.IP, prefix.PrefixLen, record)
				v6Count++
			}
		}
	}

	g.mu.Lock()
	g.trieV4 = rootV4
	g.trieV6 = rootV6
	g.stringTable = stringTable
	g.mu.Unlock()

	fmt.Printf("Loaded %d IPv4 prefixes, %d IPv6 prefixes, skipped %d rows in %v\n",
		v4Count, v6Count, skippedCount, time.Since(start))
	return nil
}

func (g *IPGeo) Lookup(ip net.IP) (string, string, string, string, float64, float64, bool) {
	if ip == nil {
		return "", "", "", "", 0, 0, false
	}
	g.mu.RLock()
	defer g.mu.RUnlock()

	if ipv4 := ip.To4(); ipv4 != nil {
		record := lookupTrie(g.trieV4, ipv4, 32)
		if record != nil {
			return g.stringTable.GetString(record.CountryCode),
				g.stringTable.GetString(record.Country),
				g.stringTable.GetString(record.Region),
				g.stringTable.GetString(record.City),
				float64(record.Lat), float64(record.Lng), true
		}
		return "", "", "", "", 0, 0, false
	}

	ip16 := ip.To16()
	if ip16 == nil {
		return "", "", "", "", 0, 0, false
	}

	// IPv6 lookup
	record := lookupTrie(g.trieV6, ip16, 128)
	if record != nil {
		return g.stringTable.GetString(record.CountryCode),
			g.stringTable.GetString(record.Country),
			g.stringTable.GetString(record.Region),
			g.stringTable.GetString(record.City),
			float64(record.Lat), float64(record.Lng), true
	}
	return "", "", "", "", 0, 0, false
}

func getCachePath() string {
	return filepath.Join(basePath, "ipgeo-cache-latest.bin")
}

func getCSVPath(year, month string) string {
	return filepath.Join(basePath, fmt.Sprintf("dbip-city-lite-%s-%s.csv.gz", year, month))
}

var (
	defaultGeo   *IPGeo
	defaultGeoMu sync.RWMutex
	initMu       sync.Mutex
)

func Init() {
	if err := InitWithError(); err != nil {
		fmt.Printf("Warning: geoip initialization failed: %v\n", err)
	}
}

func InitWithError() error {
	initMu.Lock()
	defer initMu.Unlock()

	nextGeo := New()
	now := time.Now()
	year := now.Year()
	month := int(now.Month())
	if basePath == "./.ipdata" {
		userHome, err := os.UserHomeDir()
		if err == nil {
			basePath = filepath.Join(userHome, ".ipdata")
		}
	}
	if err := os.MkdirAll(basePath, 0755); err != nil {
		fallbackPath := filepath.Join(".", ".ipdata")
		fmt.Printf("Warning: Could not create base path %s (%v), falling back to %s\n", basePath, err, fallbackPath)
		basePath = fallbackPath
		if err := os.MkdirAll(basePath, 0755); err != nil {
			return fmt.Errorf("failed to create base path %s: %w", basePath, err)
		}
	}

	monthStr := fmt.Sprintf("%02d", month)
	yearStr := fmt.Sprintf("%d", year)
	currentYM := fmt.Sprintf("%s-%s", yearStr, monthStr)

	latestPath := filepath.Join(basePath, "latest-ipdb.txt")
	cachePath := getCachePath()
	cacheLoaded := false

	// Check latest-ipdb.txt
	needUpdate := true
	if data, err := os.ReadFile(latestPath); err == nil {
		storedYM := strings.TrimSpace(string(data))
		if storedYM == currentYM {
			needUpdate = false
		}
	}

	if err := nextGeo.LoadCache(cachePath); err == nil {
		cacheLoaded = true
	} else if !os.IsNotExist(err) {
		fmt.Printf("Warning: Could not load cache: %v\n", err)
	}

	if needUpdate || !cacheLoaded {
		fmt.Printf("Cache not found or invalid, loading from CSV...\n")

		csvPath, availableYM, err := csvPathForAvailableMonth(now)
		if err != nil {
			if cacheLoaded {
				fmt.Printf("Warning: Could not refresh DB-IP data, using existing cache: %v\n", err)
				return nil
			}
			return err
		}

		// Load from CSV
		if err := nextGeo.LoadDBIP(csvPath); err != nil {
			if cacheLoaded {
				fmt.Printf("Warning: Could not load refreshed DB-IP data, using existing cache: %v\n", err)
				defaultGeoMu.Lock()
				defaultGeo = nextGeo
				defaultGeoMu.Unlock()
				return nil
			}
			return err
		}

		// Save cache
		if err := nextGeo.SaveCache(cachePath); err != nil {
			fmt.Printf("Warning: Could not save cache: %v\n", err)
		}

		// Write latest-ipdb.txt
		if err := os.WriteFile(latestPath, []byte(availableYM+"\n"), 0644); err != nil {
			fmt.Printf("Warning: Could not write latest-ipdb.txt: %v\n", err)
		}
	}
	defaultGeoMu.Lock()
	defaultGeo = nextGeo
	defaultGeoMu.Unlock()
	return nil
}

type GeoRecord struct {
	CountryCode string
	Country     string
	Region      string
	City        string
	Latitude    float64
	Longitude   float64
	Found       bool
}

func LookupNetIP(ip net.IP) GeoRecord {
	defaultGeoMu.RLock()
	g := defaultGeo
	defaultGeoMu.RUnlock()
	if g == nil {
		return GeoRecord{Found: false}
	}
	cc, country, region, city, lat, lng, ok := g.Lookup(ip)
	return GeoRecord{
		CountryCode: cc,
		Country:     country,
		Region:      region,
		City:        city,
		Latitude:    lat,
		Longitude:   lng,
		Found:       ok,
	}
}

func Lookup(ipStr string) GeoRecord {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return GeoRecord{Found: false}
	}
	return LookupNetIP(ip)
}

// countryByIP returns the ISO 3166-1 alpha-2 country code as bytes for a given IP.
func countryByIP(ip net.IP) []byte {
	if ip == nil {
		return nil
	}
	record := LookupNetIP(ip)
	if !record.Found {
		return nil
	}
	return []byte(record.CountryCode)
}

// Country returns the country code as a string for a given IP address string.
func Country(ip string) string {
	return string(countryByIP(net.ParseIP(ip)))
}

// CountryByIP returns the country code as a string for a given net.IP.
func CountryByIP(ip net.IP) string {
	return string(countryByIP(ip))
}
