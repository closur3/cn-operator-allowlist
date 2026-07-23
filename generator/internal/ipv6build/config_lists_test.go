package ipv6build

import (
	"net/netip"
	"path/filepath"
	"testing"
)

func TestRepositoryAllocationConfig(t *testing.T) {
	cfg, err := LoadAllocationConfig(filepath.Join("..", "..", "config", "ipv6-province-prefixes.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Provinces) != 31 {
		t.Fatalf("province count = %d, want 31", len(cfg.Provinces))
	}
	if len(cfg.allocations) != 328 {
		t.Fatalf("allocation count = %d, want 328", len(cfg.allocations))
	}
}

func TestBuildPublicListsCollapsesWithoutLosingProvince(t *testing.T) {
	cfg, err := LoadAllocationConfig(filepath.Join("..", "..", "config", "ipv6-province-prefixes.json"))
	if err != nil {
		t.Fatal(err)
	}
	admitted := map[string][]netip.Prefix{
		"chinatelecom": {
			netip.MustParsePrefix("240e:470::/31"),
			netip.MustParsePrefix("240e:472::/31"),
			netip.MustParsePrefix("240e:470::/32"),
		},
	}
	lists, err := BuildPublicLists(admitted, cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := []netip.Prefix{netip.MustParsePrefix("240e:470::/30")}
	if !equalPrefixes(lists.Operators["chinatelecom"], want) {
		t.Fatalf("operator list = %v, want %v", lists.Operators["chinatelecom"], want)
	}
	if !equalPrefixes(lists.Provinces["zhejiang"], want) {
		t.Fatalf("Zhejiang list = %v, want %v", lists.Provinces["zhejiang"], want)
	}
	if !equalPrefixes(lists.CN, want) {
		t.Fatalf("CN list = %v, want %v", lists.CN, want)
	}
}

func TestVerifyPublicLists(t *testing.T) {
	cfg, err := LoadAllocationConfig(filepath.Join("..", "..", "config", "ipv6-province-prefixes.json"))
	if err != nil {
		t.Fatal(err)
	}
	lists, err := BuildPublicLists(map[string][]netip.Prefix{
		"chinatelecom": {netip.MustParsePrefix("240e:470::/30")},
	}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := WritePublicLists(dir, lists); err != nil {
		t.Fatal(err)
	}
	if err := VerifyPublicLists(dir, cfg); err != nil {
		t.Fatal(err)
	}
}
