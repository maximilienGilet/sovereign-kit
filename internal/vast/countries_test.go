package vast

import (
	"strings"
	"testing"
)

func TestCountryCatalogExcludesRetiredCodesAndEmptyNames(t *testing.T) {
	catalog := Countries()
	if len(catalog) != 249 {
		t.Errorf("country count=%d want 249 assigned ISO countries", len(catalog))
	}
	for _, country := range catalog {
		if strings.TrimSpace(country.Name) == "" {
			t.Errorf("blank country name: %q", country.Code)
		}
	}
	for _, code := range []string{"FQ", "YU", "CS", "AN", "SU", "NT", "PC"} {
		if _, err := NormalizeCountries([]string{code}); err == nil {
			t.Errorf("accepted retired code %s", code)
		}
	}
	if code := CountryCode(""); code != "" {
		t.Errorf("empty location invented country %s", code)
	}
}
