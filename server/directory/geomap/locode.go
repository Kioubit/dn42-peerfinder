package geomap

import (
	_ "embed"
	"encoding/json"
	"strings"
)

// loadCountryPolygons parses a FeatureCollection of country boundaries and extracts properties safely.
func loadCountryPolygons(raw []byte, geomOut map[string]json.RawMessage, nameOut map[string]string) error {
	var fc struct {
		Features []struct {
			ID         string          `json:"id"`
			Geometry   json.RawMessage `json:"geometry"`
			Properties struct {
				Name string `json:"name"`
			} `json:"properties"`
		} `json:"features"`
	}
	if err := json.Unmarshal(raw, &fc); err != nil {
		return err
	}
	for _, f := range fc.Features {
		id := strings.ToUpper(strings.TrimSpace(f.ID))
		if id == "" {
			continue
		}
		if len(f.Geometry) > 0 {
			geomOut[id] = f.Geometry
		}
		if f.Properties.Name != "" {
			nameOut[id] = f.Properties.Name
		}
	}
	return nil
}

// loCodeLonLat returns the GeoJSON order [lat, lon] for the given loCode, or
// false if unknown.
func (c *LoCodeBuilder) loCodeLatLon(loCode string) ([2]float64, bool) {
	coords, ok := c.loCodeEntries[normalizeLoCode(loCode)]
	if !ok {
		return coords, false
	}
	return coords, true
}

// countryName returns the canonical country name for an ISO alpha-2 code.
func (c *LoCodeBuilder) countryName(cc string) string {
	return c.countryNames[cc]
}

// countryPolygon returns the GeoJSON geometry for an ISO alpha-2 code.
func (c *LoCodeBuilder) countryPolygon(cc string) (json.RawMessage, bool) {
	g, ok := c.countryPolygons[cc]
	return g, ok
}

// pointGeometry constructs a GeoJSON point coordinates RawMessage.
// Note that in GeoJSON, the coordinate order is [lon, lat]
func pointGeometry(lat, lon float64) json.RawMessage {
	g, _ := json.Marshal(geometryPoint{Type: "Point", Coordinates: [2]float64{lon, lat}})
	return g
}

// normalizeLoCode returns the format required for the loCode directory keys.
func normalizeLoCode(loCode string) string {
	return strings.ToUpper(strings.Join(strings.Fields(loCode), ""))
}

// splitLoCode splits a "<COUNTRY><PLACE>" loCode into its country (alpha-2) and
// place components. The UN/LOCODE country is always two characters.
func splitLoCode(loCode string) (country, city string) {
	if len(loCode) < 2 {
		return loCode, ""
	}
	return loCode[:2], loCode[2:]
}
