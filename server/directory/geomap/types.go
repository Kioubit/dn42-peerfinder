package geomap

import (
	_ "embed"
	"encoding/json"
)

//go:embed data-assets/generated/locodes.json
var loCodesJSON []byte

//go:embed data-assets/generated/countries.geo.json
var countriesGeoJSON []byte

type LoCodeBuilder struct {
	cityCounts    map[string]int
	countryCounts map[string]int

	// ---- Metadata sources ----
	// loCodeEntries maps an uppercase UN/LOCODE ("<COUNTRY><PLACE>", e.g. "USSEA")
	// to its decimal-degree coordinates and place name. Each value is the array
	// [latitude, longitude].
	loCodeEntries map[string][2]float64
	// countryNames maps an ISO-3166 alpha-2 country code to its canonical name.
	countryNames map[string]string
	// countryPolygons maps an ISO-3166 alpha-2 country code to its GeoJSON geometry
	// (Polygon/MultiPolygon), used to highlight countries on the map.
	countryPolygons map[string]json.RawMessage
}

// mapDataResponse is a GeoJSON FeatureCollection mixing point features (cities
// with a resolvable UN/LOCODE) and polygon features (countries that have
// servers)
type mapDataResponse struct {
	Type     string       `json:"type"`
	Features []mapFeature `json:"features"`
}

type mapFeature struct {
	Type       string            `json:"type"`
	Geometry   json.RawMessage   `json:"geometry"`
	Properties featureProperties `json:"properties"`
}

type geometryPoint struct {
	Type        string     `json:"type"`
	Coordinates [2]float64 `json:"coordinates"`
}

type featureProperties struct {
	Kind        string `json:"kind"` // "city" or "country"
	Locode      string `json:"locode,omitempty"`
	Country     string `json:"country,omitempty"`
	CountryName string `json:"countryName,omitempty"`
	CityName    string `json:"cityName,omitempty"`
	HasCities   *bool  `json:"hasCities,omitempty"` // pointer so *false is output explicitly as false
	Count       int    `json:"count"`
}
