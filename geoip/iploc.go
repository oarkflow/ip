package geoip

import (
	"bufio"
	"compress/gzip"
	"encoding/binary"
	"encoding/csv"
	"encoding/gob"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var basePath = "./data"

func SetBasePath(path string) {
	basePath = path
}

type StringTable struct {
	strings []string
	lookup  map[string]uint16
	mu      sync.RWMutex
}

func NewStringTable() *StringTable {
	return &StringTable{
		strings: make([]string, 1), // Index 0 = empty string
		lookup:  make(map[string]uint16),
	}
}

func (st *StringTable) GetIndex(s string) uint16 {
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

	idx := uint16(len(st.strings))
	st.strings = append(st.strings, s)
	st.lookup[s] = idx
	return idx
}

func (st *StringTable) GetString(idx uint16) string {
	st.mu.RLock()
	defer st.mu.RUnlock()
	if int(idx) >= len(st.strings) {
		return ""
	}
	return st.strings[idx]
}

type TrieRecord struct {
	CountryCode uint16
	Country     uint16
	Region      uint16
	City        uint16
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
	cacheVersion = 2
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

func getDBUrl(year, month string) string {
	return fmt.Sprintf("https://download.db-ip.com/free/dbip-city-lite-%s-%s.csv.gz", year, month)
}

func download(url, filepath string) error {
	fmt.Printf("Downloading %s...\n", url)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func (g *IPGeo) SaveCache(cachePath string) error {
	file, err := os.Create(cachePath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

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

	g.stringTable = NewStringTable()
	g.stringTable.strings = make([]string, stringCount)
	g.stringTable.lookup = make(map[string]uint16)

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
		g.stringTable.strings[i] = str
		if str != "" {
			g.stringTable.lookup[str] = uint16(i)
		}
	}

	// Read tries using gob
	decoder := gob.NewDecoder(reader)
	if err := decoder.Decode(&g.trieV4); err != nil {
		return err
	}
	if err := decoder.Decode(&g.trieV6); err != nil {
		return err
	}

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

	// Read all records
	var records [][]string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		record := make([]string, len(rec))
		copy(record, rec)
		records = append(records, record)
	}

	// Build tries
	rootV4 := &TrieNode{}
	rootV6 := &TrieNode{}
	v4Count := 0
	v6Count := 0

	for _, rec := range records {
		if len(rec) < 8 {
			continue
		}

		startIP := strings.TrimSpace(rec[0])
		endIP := strings.TrimSpace(rec[1])
		cc := g.stringTable.GetIndex(rec[2])
		country := g.stringTable.GetIndex(rec[3])
		region := g.stringTable.GetIndex(rec[4])
		city := g.stringTable.GetIndex(rec[5])

		lat, _ := strconv.ParseFloat(rec[6], 32)
		lng, _ := strconv.ParseFloat(rec[7], 32)

		sip := net.ParseIP(startIP)
		eip := net.ParseIP(endIP)
		if sip == nil || eip == nil {
			continue
		}

		prefixLen, err := computePrefixLen(sip, eip)
		if err != nil {
			continue // Skip if not exact CIDR
		}

		record := &TrieRecord{
			CountryCode: cc,
			Country:     country,
			Region:      region,
			City:        city,
			Lat:         float32(lat),
			Lng:         float32(lng),
		}

		if sip.To4() != nil {
			insertTrie(rootV4, sip.To4(), prefixLen, record)
			v4Count++
		} else {
			insertTrie(rootV6, sip, prefixLen, record)
			v6Count++
		}
	}

	g.mu.Lock()
	g.trieV4 = rootV4
	g.trieV6 = rootV6
	g.mu.Unlock()

	fmt.Printf("Loaded %d IPv4 entries, %d IPv6 entries in %v\n",
		v4Count, v6Count, time.Since(start))
	return nil
}

func (g *IPGeo) Lookup(ip net.IP) (string, string, string, string, float64, float64, bool) {
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

	// IPv6 lookup
	record := lookupTrie(g.trieV6, ip.To16(), 128)
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

var defaultGeo *IPGeo

func init() {
	defaultGeo = New()
	now := time.Now()
	year := now.Year()
	month := int(now.Month())

	os.MkdirAll(basePath, 0755)

	monthStr := fmt.Sprintf("%02d", month)
	yearStr := fmt.Sprintf("%d", year)
	currentYM := fmt.Sprintf("%s-%s", yearStr, monthStr)

	latestPath := filepath.Join(basePath, "latest-ipdb.txt")
	cachePath := getCachePath()
	csvPath := getCSVPath(yearStr, monthStr)

	// Check latest-ipdb.txt
	needUpdate := true
	if data, err := os.ReadFile(latestPath); err == nil {
		storedYM := strings.TrimSpace(string(data))
		if storedYM == currentYM {
			needUpdate = false
		}
	}

	if !needUpdate {
		if err := defaultGeo.LoadCache(cachePath); err == nil {
			fmt.Println("Loaded from cache successfully!")
		} else {
			needUpdate = true
		}
	}

	if needUpdate {
		fmt.Printf("Cache not found or invalid, loading from CSV...\n")

		// Check if CSV file exists, if not download it
		if _, err := os.Stat(csvPath); os.IsNotExist(err) {
			url := getDBUrl(yearStr, monthStr)
			if err := download(url, csvPath); err != nil {
				panic(err)
			}
		}

		// Load from CSV
		if err := defaultGeo.LoadDBIP(csvPath); err != nil {
			panic(err)
		}

		// Save cache
		fmt.Println("Saving cache...")
		if err := defaultGeo.SaveCache(cachePath); err != nil {
			fmt.Printf("Warning: Could not save cache: %v\n", err)
		} else {
			fmt.Println("Cache saved successfully!")
		}

		// Write latest-ipdb.txt
		if err := os.WriteFile(latestPath, []byte(currentYM+"\n"), 0644); err != nil {
			fmt.Printf("Warning: Could not write latest-ipdb.txt: %v\n", err)
		}
	}

	fmt.Println("Initialization complete")
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
	cc, country, region, city, lat, lng, ok := defaultGeo.Lookup(ip)
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
