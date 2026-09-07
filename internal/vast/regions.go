package vast

import (
	"fmt"
	"golang.org/x/text/language"
)

type GeographicRegion struct{ ID, Name string }

// Region membership comes from the same installed CLDR data as Countries.
// North America includes Central America and the Caribbean (UN M49 003).
func Regions() []GeographicRegion {
	return []GeographicRegion{{"", "World"}, {"north-america", "North America"}, {"south-america", "South America"}, {"europe", "Europe"}, {"asia", "Asia"}, {"africa", "Africa"}, {"oceania", "Oceania"}}
}

func RegionCountries(id string) ([]Country, error) {
	if id == "" {
		return Countries(), nil
	}
	codes := map[string]string{"north-america": "003", "south-america": "005", "europe": "150", "asia": "142", "africa": "002", "oceania": "009"}
	code, ok := codes[id]
	if !ok {
		return nil, fmt.Errorf("invalid geographic region %q", id)
	}
	region := language.MustParseRegion(code)
	var countries []Country
	for _, country := range countryCatalog {
		if region.Contains(language.MustParseRegion(country.Code)) {
			countries = append(countries, country)
		}
	}
	if len(countries) == 0 {
		return nil, fmt.Errorf("no country data for region %q", id)
	}
	return countries, nil
}

// GeographicCountries expands a whole region, or validates an explicit country
// refinement. Never turn a disjoint region/country selection into a world query.
func GeographicCountries(region string, codes []string) ([]string, error) {
	catalog, err := RegionCountries(region)
	if err != nil {
		return nil, err
	}
	normalized, err := NormalizeCountries(codes)
	if err != nil {
		return nil, err
	}
	if region == "" {
		return normalized, nil
	}
	allowed := make(map[string]bool, len(catalog))
	var expanded []string
	for _, country := range catalog {
		allowed[country.Code] = true
		expanded = append(expanded, country.Code)
	}
	if len(normalized) == 0 {
		return expanded, nil
	}
	for _, code := range normalized {
		if !allowed[code] {
			return nil, fmt.Errorf("country %s is outside the selected region", code)
		}
	}
	return normalized, nil
}

func RegionName(id string) string {
	for _, region := range Regions() {
		if region.ID == id {
			return region.Name
		}
	}
	return "Unknown region"
}
