package vast

import "testing"

func TestRegionsUseGeographicCountryMembership(t *testing.T) {
	for _, tc := range []struct{ region, included, excluded string }{
		{"north-america", "MX", "BR"}, {"north-america", "JM", "AR"},
		{"south-america", "BR", "US"}, {"europe", "FR", "JP"},
		{"asia", "JP", "ZA"}, {"africa", "ZA", "AU"}, {"oceania", "AU", "FR"},
	} {
		countries, err := RegionCountries(tc.region)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, country := range countries {
			if country.Code == tc.included {
				found = true
			}
			if country.Code == tc.excluded {
				t.Fatalf("%s contains %s", tc.region, tc.excluded)
			}
		}
		if !found {
			t.Fatalf("%s missing %s", tc.region, tc.included)
		}
	}
	if countries, err := RegionCountries(""); err != nil || len(countries) != len(Countries()) {
		t.Fatal("world must retain full catalog")
	}
	if _, err := RegionCountries("bogus"); err == nil {
		t.Fatal("invalid region accepted")
	}
}
