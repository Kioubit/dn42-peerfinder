package geomap

import (
	"encoding/json"
	"peerfinder-db/directory/directoryTypes"
	"sort"
	"strings"
)

func NewLoCodeBuilder() *LoCodeBuilder {
	builder := LoCodeBuilder{
		cityCounts:    make(map[string]int),
		countryCounts: make(map[string]int),
	}

	builder.loCodeEntries = make(map[string][2]float64)
	if err := json.Unmarshal(loCodesJSON, &builder.loCodeEntries); err != nil {
		panic("failed to load embedded locodes.json: " + err.Error())
	}

	builder.countryPolygons = make(map[string]json.RawMessage)
	builder.countryNames = make(map[string]string)
	if err := loadCountryPolygons(countriesGeoJSON, builder.countryPolygons, builder.countryNames); err != nil {
		panic("failed to load embedded countries.geo.json: " + err.Error())
	}

	return &builder
}

func (c *LoCodeBuilder) AddServersSingleNetwork(servers directoryTypes.YamlServers) {
	seenLoCodes := make(map[string]struct{})
	for _, srv := range servers {
		cc := strings.ToUpper(strings.TrimSpace(srv.CountryCode))
		if cc == "" {
			continue
		}
		city := strings.ToUpper(strings.TrimSpace(srv.City))
		if city != "" {
			loCode := cc + city
			if _, seen := seenLoCodes[loCode]; seen {
				// Only count a presence once per network per city (but always count it on a country level)
				continue
			}
			seenLoCodes[loCode] = struct{}{}
			c.cityCounts[loCode]++
		}
		c.countryCounts[cc]++
	}
}

// Build returns the fully built, sorted GeoJSON FeatureCollection
func (c *LoCodeBuilder) Build() []byte {
	features := make([]mapFeature, 0, len(c.cityCounts)+len(c.countryCounts))
	countryHasCities := make(map[string]bool)

	// Append city Polygon features
	for loCode, count := range c.cityCounts {
		country, _ := splitLoCode(loCode)

		coord, ok := c.loCodeLatLon(loCode)
		if !ok {
			continue // Skip unresolvable city coordinates
		}

		features = append(features, mapFeature{
			Type:     "Feature",
			Geometry: pointGeometry(coord[0], coord[1]),
			Properties: featureProperties{
				Kind:        "city",
				Locode:      loCode,
				CountryName: c.countryName(country),
				CityName:    loCode,
				Count:       count,
			},
		})
		countryHasCities[country] = true
	}

	// Append country Polygon features
	for country, count := range c.countryCounts {
		geom, ok := c.countryPolygon(country)
		if !ok {
			continue
		}
		features = append(features, mapFeature{
			Type:     "Feature",
			Geometry: geom,
			Properties: featureProperties{
				Kind:        "country",
				Country:     country,
				CountryName: c.countryName(country),
				HasCities:   new(countryHasCities[country]),
				Count:       count,
			},
		})
	}

	// Sort elements for stable responses and deterministic ETags
	sort.Slice(features, func(i, j int) bool {
		pi, pj := features[i].Properties, features[j].Properties
		if pi.Kind != pj.Kind {
			return pi.Kind < pj.Kind // Cities sorted before country boundaries
		}
		if pi.Country != pj.Country {
			return pi.Country < pj.Country
		}
		return pi.Locode < pj.Locode
	})

	resp := mapDataResponse{
		Type:     "FeatureCollection",
		Features: features,
	}

	b, _ := json.Marshal(&resp)

	return b
}
