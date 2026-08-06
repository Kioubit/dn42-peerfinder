package directory

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"log"
	"maps"
	"math/rand/v2"
	"path/filepath"
	"peerfinder-db/directory/directoryTypes"
	"peerfinder-db/directory/geomap"
	"peerfinder-db/directory/interner"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"peerfinder-db/config"
)

// listCacheTTL controls how often the in-memory network index is refreshed from
// disk. The node-directory data can be updated externally (e.g. a git sync), so
// we lazily rebuild the index after this interval instead of stat-ing all
// files on every request. Writes through the API invalidate the cache
// immediately (see ListCache.invalidate)
const listCacheTTL = 24 * time.Hour

// cachedNetwork holds a parsed network *index* in memory. The heavy per-network
// payload (the Servers map) is intentionally dropped from the cached copy so the
// backend never holds the entire directory's bodies in memory at once. The
// server count is retained so the list can show how many servers a network has
// without a further disk read; the full Servers bodies are fetched on demand
// via readFullNetwork / the /api/servers endpoint.
type cachedNetwork struct {
	net         directoryTypes.YamlNetwork
	asn         string // ASN derived from the filename (e.g. "4242421088")
	serverCount int
	locations   []NetworkLocation
}

type NetworkLocation struct {
	Country string `json:"country"`
	City    string `json:"city"`
}

// ListCache is a single, shared, read-mostly snapshot of the network *index*.
//
// Only the lightweight index (no Servers map) is kept in memory. This bounds RAM
// to a small multiple of the network count rather than the full dataset, while
// keeping request-time filtering allocation-free. Full bodies are
// fetched from disk per-ASN when actually needed
type ListCache struct {
	mu           sync.RWMutex
	entries      []cachedNetwork
	allCountries []string
	etag         string
	builtAt      time.Time

	mapDataJSON []byte
	building    sync.Mutex
}

func (c *ListCache) GetEtag() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.etag
}

func NewNetworkListCache() *ListCache {
	return &ListCache{}
}

// Get returns a shared, read-only snapshot of the cached index entries and the
// data ETag, rebuilding the index first if it is missing or stale. Callers MUST
// NOT mutate the returned slice. The YamlNetwork.Servers field is always nil on entries
// from this cache; use readFullNetwork for the full body.
func (c *ListCache) Get() ([]cachedNetwork, []string, string) {
	c.mu.RLock()
	if c.entries != nil && time.Since(c.builtAt) < listCacheTTL {
		entries, allCountries, etag := c.entries, c.allCountries, c.etag
		c.mu.RUnlock()
		return entries, allCountries, etag
	}
	c.mu.RUnlock()
	return c.rebuild()
}

// invalidate forces the next Get() to rebuild the index from disk. Call this
// whenever the underlying data directory is modified through the API.
func (c *ListCache) invalidate() {
	c.mu.Lock()
	c.builtAt = time.Time{}
	c.mu.Unlock()
}

func (c *ListCache) rebuild() ([]cachedNetwork, []string, string) {
	// Only one goroutine walks the disk at a time. Concurrent callers wait and
	// then reuse the freshly built snapshot.
	c.building.Lock()
	defer c.building.Unlock()

	// Another goroutine may have rebuilt while we waited for the lock.
	c.mu.RLock()
	if c.entries != nil && time.Since(c.builtAt) < listCacheTTL {
		entries, allCountries, etag := c.entries, c.allCountries, c.etag
		c.mu.RUnlock()
		return entries, allCountries, etag
	}
	c.mu.RUnlock()

	loCodeBuilder := geomap.NewLoCodeBuilder()

	entries := make([]cachedNetwork, 0, 2048)
	h := sha256.New()

	seenCountries := make(map[string]struct{})
	sIn := make(interner.StringInterner)

	_ = filepath.WalkDir(config.Global.DataDirectory, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		nw, modTime, err := ReadYAMLFile(config.Global.DataDirectory, d.Name())
		if err != nil {
			log.Printf("Error reading %s: %v", d.Name(), err)
			return nil
		}
		// Fold filename + modTime into the ETag so it changes when data changes
		// but stays stable across rebuilds when nothing changed.
		_, _ = h.Write([]byte(d.Name()))
		_, _ = h.Write([]byte(strconv.FormatInt(modTime.UnixNano(), 16)))

		asn := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
		count := len(nw.Servers)
		countries := distinctCountryCodes(nw.Servers, sIn)
		for _, country := range countries {
			seenCountries[country] = struct{}{}
		}

		loCodeBuilder.AddServersSingleNetwork(nw.Servers)
		locations := distinctLocations(nw.Servers, sIn)

		nw.Servers = nil
		entries = append(entries, cachedNetwork{
			net:         *nw,
			asn:         asn,
			serverCount: count,
			locations:   locations,
		})
		return nil
	})

	etag := `"` + hex.EncodeToString(h.Sum(nil)[:16]) + `"`

	rand.Shuffle(len(entries), func(i, j int) { entries[i], entries[j] = entries[j], entries[i] })

	allCountries := make([]string, 0, len(seenCountries))
	for country := range seenCountries {
		allCountries = append(allCountries, country)
	}
	sort.Strings(allCountries)

	c.mu.Lock()
	c.entries = entries
	c.allCountries = allCountries
	c.etag = etag
	c.builtAt = time.Now()
	c.mapDataJSON = loCodeBuilder.Build()
	c.mu.Unlock()

	return entries, allCountries, etag
}

// readFullNetwork reads and parses the complete YAML for a single ASN,
// including the heavy Servers map, directly from disk.
func readFullNetwork(asn string) (*directoryTypes.YamlNetwork, time.Time, error) {
	if _, err := strconv.ParseUint(asn, 10, 32); err != nil {
		return nil, time.Time{}, errors.New("invalid ASN value")
	}
	name := asn + ".yml"
	return ReadYAMLFile(config.Global.DataDirectory, name)
}

// distinctCountryCodes returns the set of country codes
// present across the given servers. Empty codes are skipped.
func distinctCountryCodes(servers directoryTypes.YamlServers, sIn interner.StringInterner) []string {
	seen := make(map[string]struct{})
	for _, s := range servers {
		code := sIn.Intern(strings.ToUpper(strings.TrimSpace(s.CountryCode)))
		if code == "" {
			continue
		}
		seen[code] = struct{}{}
	}
	if len(seen) == 0 {
		return []string{}
	}
	return slices.Collect(maps.Keys(seen))
}

// distinctLocations returns deduplicated unique locations per network
func distinctLocations(servers directoryTypes.YamlServers, sIn interner.StringInterner) []NetworkLocation {
	type key struct {
		country string
		city    string
	}
	seen := make(map[key]struct{})
	for _, s := range servers {
		country := sIn.Intern(strings.ToUpper(strings.TrimSpace(s.CountryCode)))
		city := sIn.Intern(strings.TrimSpace(s.City))
		if country == "" && city == "" {
			continue
		}
		seen[key{country: country, city: city}] = struct{}{}
	}
	if len(seen) == 0 {
		return nil
	}
	locs := make([]NetworkLocation, 0, len(seen))
	for k := range seen {
		locs = append(locs, NetworkLocation{
			Country: k.country,
			City:    k.city,
		})
	}
	// Keep internal lists consistently sorted
	sort.Slice(locs, func(i, j int) bool {
		if locs[i].Country != locs[j].Country {
			return locs[i].Country < locs[j].Country
		}
		return locs[i].City < locs[j].City
	})
	return locs
}
