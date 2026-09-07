package vast

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

type Country struct{ Code, Name string }

// The installed language data provides countries independently of offer results.
var countryCatalog = func() []Country {
	var countries []Country
	for _, region := range language.Supported.Regions() {
		// CLDR also labels these exceptionally reserved subdivisions as
		// countries; they are not assigned ISO 3166-1 alpha-2 countries.
		switch region.String() {
		case "AC", "CP", "DG", "EA", "IC", "TA", "FQ", "YU", "CS", "AN", "SU", "NT", "PC":
			continue
		}
		if len(region.String()) == 2 && region.IsCountry() && !region.IsPrivateUse() && region.ISO3() != "ZZZ" && region.Canonicalize() == region {
			name := strings.TrimSpace(display.English.Regions().Name(region))
			// Multi-successor retired regions have no display name but can pass
			// Canonicalize/IsCountry in the installed CLDR data.
			if name != "" {
				countries = append(countries, Country{Code: region.String(), Name: name})
			}
		}
	}
	sort.Slice(countries, func(i, j int) bool { return countries[i].Name < countries[j].Name })
	return countries
}()

func Countries() []Country { return append([]Country(nil), countryCatalog...) }

func NormalizeCountries(codes []string) ([]string, error) {
	known := make(map[string]bool, len(countryCatalog))
	for _, country := range countryCatalog {
		known[country.Code] = true
	}
	var normalized []string
	seen := make(map[string]bool, len(codes))
	for _, code := range codes {
		code = strings.ToUpper(strings.TrimSpace(code))
		if !known[code] {
			return nil, fmt.Errorf("invalid country code %q", code)
		}
		if !seen[code] {
			normalized = append(normalized, code)
			seen[code] = true
		}
	}
	return normalized, nil
}

// CountryCode accepts a country code or the provider's comma-separated location
// suffix. A location without an identifiable country remains unknown.
func CountryCode(location string) string {
	parts := strings.Split(location, ",")
	last := strings.TrimSpace(parts[len(parts)-1])
	if last == "" {
		return ""
	}
	for _, country := range countryCatalog {
		if strings.EqualFold(last, country.Code) || strings.EqualFold(last, country.Name) {
			return country.Code
		}
	}
	return ""
}
