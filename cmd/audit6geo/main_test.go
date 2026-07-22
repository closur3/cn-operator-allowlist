package main

import (
	"net/netip"
	"testing"
)

func TestPurposeForPrefixRequiresOnePurpose(t *testing.T) {
	parents := []admissionParent{
		{netip.MustParsePrefix("240e:300::/24"), "fixed_broadband"},
		{netip.MustParsePrefix("240e:400::/24"), "mobile"},
	}
	if got, ok := purposeForPrefix(netip.MustParsePrefix("240e:300:12::/48"), parents); !ok || got != "fixed_broadband" { t.Fatalf("got %q %v", got, ok) }
	if _, ok := purposeForPrefix(netip.MustParsePrefix("240e:500::/32"), parents); ok { t.Fatal("outside prefix was classified") }
}

func TestSlash32EquivalentSupportsOriginalBroadAndSpecificPrefixes(t *testing.T) {
	got := slash32Equivalent([]netip.Prefix{
		netip.MustParsePrefix("240e:300::/30"),
		netip.MustParsePrefix("240e:400::/33"),
	})
	if got != "4.500000000" { t.Fatalf("got %s", got) }
}

func TestProvinceNameNormalization(t *testing.T) {
	for _, value := range []string{"Zhejiang", "Zhejiang Sheng", "Zhejiang Province"} {
		got, ok := provinceForRegion(value)
		if !ok || got.Code != "CN-ZJ" { t.Fatalf("%q: got %+v %v", value, got, ok) }
	}
}

func TestProvinceConflictIsNotForced(t *testing.T) {
	_, reason := classifyProvince(geoRecord{Country: "CN", Region: "Zhejiang"}, geoRecord{Country: "CN", Region: "Jiangsu"})
	if reason != "province_conflict_within_unit" { t.Fatalf("got %q", reason) }
}
