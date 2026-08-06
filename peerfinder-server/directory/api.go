package directory

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"peerfinder-db/config"
	"peerfinder-db/directory/directoryTypes"
	"peerfinder-db/kauth"
	"strconv"
	"strings"
)

const defaultPageSize = 5
const maxPageSize = 50

// listResponse is the paginated envelope returned by GetListHandler.
//
// Each item is a wrapped directoryTypes.YamlNetwork whose Servers map is omitted. Clients fetch
// the full server list on demand via /api/servers. ServerCount is included so
// the UI can show how many servers a network has without an extra request.
type listResponse struct {
	Items []listResponseNetwork `json:"items"`
	Total int                   `json:"total"`
}

type listResponseNetwork struct {
	directoryTypes.YamlNetwork
	ASN         string `json:"asn"`
	ServerCount int    `json:"serverCount"`
}

// GetListHandler serves a filtered, paginated slice of the network directory.
func (c *ListCache) GetListHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	page := max(parseIntDefault(q.Get("page"), 1), 1)
	pageSize := parseIntDefault(q.Get("pageSize"), defaultPageSize)
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	search := strings.ToLower(strings.TrimSpace(q.Get("q")))
	country := strings.ToUpper(strings.TrimSpace(q.Get("country")))
	city := strings.ToUpper(strings.TrimSpace(q.Get("city")))
	var tags []string
	if t := strings.TrimSpace(q.Get("tags")); t != "" {
		tags = strings.Split(t, ",")
	}

	entries, _, dataETag := c.Get()

	// Cache headers: the response is fully determined by the data version plus the
	// query, so a matching If-None-Match lets us skip regenerating the body.
	etag := makeListETag(dataETag, search, tags, country, page, pageSize)
	w.Header().Set("ETag", "W/"+etag)
	w.Header().Set("Cache-Control", "public, max-age=43200, must-revalidate")
	if match := r.Header.Get("If-None-Match"); strings.TrimPrefix(match, "W/") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// Filter first, collecting matching indices to avoid copying entries around.
	matched := make([]int, 0, len(entries))
	for i := range entries {
		e := &entries[i]

		if search != "" {
			contains := false
			for _, searchField := range []string{e.net.Name, e.net.Mnt, e.asn, e.net.Description} {
				if strings.Contains(strings.ToLower(searchField), strings.ToLower(search)) {
					contains = true
					break
				}
			}
			if !contains {
				continue
			}
		}

		if len(tags) > 0 && !hasAllTags(e.net.Tags, tags) {
			continue
		}
		// Country filter matches networks that have at least one server in the requested country
		if country != "" {
			found := false
			for _, location := range e.locations {
				if location.Country == country {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		if city != "" && country != "" {
			found := false
			for _, loc := range e.locations {
				// loc.Country == country is checked above
				if loc.City == city {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		matched = append(matched, i)
	}
	total := len(matched)

	// Extract the requested page from the index. the count is carried separately so the list
	// can show server totals without a per-row disk read.
	start := min((page-1)*pageSize, total)
	end := min(start+pageSize, total)

	items := make([]listResponseNetwork, 0, end-start)
	for _, idx := range matched[start:end] {
		items = append(items, listResponseNetwork{
			YamlNetwork: entries[idx].net,
			ASN:         entries[idx].asn,
			ServerCount: entries[idx].serverCount,
		})
	}

	resp := listResponse{
		Items: items,
		Total: total,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(&resp)
}

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

// hasAllTags reports whether itemTags contains every requested tag.
func hasAllTags(itemTags directoryTypes.YamlTags, want []string) bool {
	for _, wt := range want {
		found := false
		for _, it := range itemTags {
			if string(it) == wt {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// withNetworkETag handles the common boilerplate shared by GetASNHandler and
// GetServersHandler: parse the ASN, load the network, set ETag/Cache-Control
// headers, serve a 304 when the ETag matches and otherwise encode the
// payload produced by payloadFn as JSON.
func withNetworkETag(w http.ResponseWriter, r *http.Request, payloadFn func(nw *directoryTypes.YamlNetwork) any) {
	asn := r.PathValue("asn")
	if asn == "" {
		http.Error(w, "asn is required", http.StatusBadRequest)
		return
	}

	nw, modTime, err := readFullNetwork(asn)
	if err != nil {
		http.Error(w, "network not found", http.StatusNotFound)
		return
	}

	etag := `"` + strconv.FormatInt(modTime.UnixNano(), 16) + `"`
	w.Header().Set("ETag", "W/"+etag)
	w.Header().Set("Cache-Control", "public, max-age=900, must-revalidate")
	if match := r.Header.Get("If-None-Match"); strings.TrimPrefix(match, "W/") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payloadFn(nw))
}

func GetASNHandler(w http.ResponseWriter, r *http.Request) {
	withNetworkETag(w, r, func(nw *directoryTypes.YamlNetwork) any { return nw })
}

func GetServersHandler(w http.ResponseWriter, r *http.Request) {
	withNetworkETag(w, r, func(nw *directoryTypes.YamlNetwork) any { return nw.Servers })
}

// makeListETag derives a compact ETag from the data version and the request
// parameters that influence the response body.
func makeListETag(dataETag, search string, tags []string, country string, page, pageSize int) string {
	h := fnv.New64a()
	_, _ = io.WriteString(h, dataETag)
	_, _ = io.WriteString(h, "|"+search+"|"+strings.Join(tags, ",")+"|"+country+"|")
	_, _ = fmt.Fprintf(h, "%d|%d", page, pageSize)
	return `"` + strconv.FormatUint(h.Sum64(), 16) + `"`
}

func (c *ListCache) DownloadLocalFinderScript(w http.ResponseWriter, r *http.Request) {
	file, err := os.Open(config.Global.ZipPath)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	defer func() {
		_ = file.Close()
	}()

	stat, err := file.Stat()
	if err != nil {
		http.Error(w, "Error reading file", http.StatusInternalServerError)
		return
	}

	w.Header().Set("ETag", c.GetEtag())
	w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d must-revalidate", 9000))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(config.Global.ZipPath))
	http.ServeContent(w, r, filepath.Base(config.Global.ZipPath), stat.ModTime(), file)
}

func GetSelfHandler(w http.ResponseWriter, _ *http.Request, session *kauth.AuthenticationInfo) {
	d, _, err := ReadYAMLFile(config.Global.DataDirectory, session.ASN+".yml")
	if err != nil {
		if errors.Is(err, errYAMLFileNotFound) {
			d = &directoryTypes.YamlNetwork{}
		} else {
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	d.Mnt = session.EffectiveMnt

	result, err := json.Marshal(&d)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(result)
}

func (c *ListCache) DeleteHandler(w http.ResponseWriter, _ *http.Request, session *kauth.AuthenticationInfo) {
	if err := deleteYAMLFile(config.Global.DataDirectory, session.ASN+".yml"); err != nil {
		log.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	c.invalidate()
}

func (c *ListCache) EditHandler(w http.ResponseWriter, r *http.Request, session *kauth.AuthenticationInfo) {
	var n directoryTypes.YamlNetwork
	lr := http.MaxBytesReader(w, r.Body, 10000)
	defer func() { _ = lr.Close() }()
	if err := json.NewDecoder(lr).Decode(&n); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	n.Mnt = session.EffectiveMnt

	if err := n.Validate(); err != nil {
		http.Error(w, "Invalid data: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Write YAML file
	if err := writeYAMLFile(config.Global.DataDirectory, session.ASN+".yml", n); err != nil {
		log.Println("Failed to write YAML file:", err)
		http.Error(w, "Failed to write YAML", http.StatusInternalServerError)
		return
	}

	// Data changed on disk; drop the cached list so the next read reflects it.
	c.invalidate()
}

// GetMapDataHandler serves a GeoJSON FeatureCollection with a point per LOCODE
// that has servers and a resolvable coordinate, plus a highlighted polygon per
// country that has servers.
func (c *ListCache) GetMapDataHandler(w http.ResponseWriter, r *http.Request) {
	etag := c.GetEtag()
	w.Header().Set("ETag", "W/"+etag)
	w.Header().Set("Cache-Control", "public, max-age=43200, must-revalidate")
	if match := r.Header.Get("If-None-Match"); strings.TrimPrefix(match, "W/") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(c.mapDataJSON)
}

func (c *ListCache) GetCountriesHandler(w http.ResponseWriter, r *http.Request) {
	etag := c.GetEtag()
	w.Header().Set("ETag", "W/"+etag)
	w.Header().Set("Cache-Control", "public, max-age=43200, must-revalidate")
	if match := r.Header.Get("If-None-Match"); strings.TrimPrefix(match, "W/") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	_, allCountries, _ := c.Get()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"countries": allCountries,
	})
}
