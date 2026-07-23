package riswhois6

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"net/netip"
	"os"
	"sort"
	"strconv"
	"strings"
)

type Record struct {
	Prefix  netip.Prefix
	Origins []string
}

func Parse(path string) ([]Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer z.Close()
	byPrefix := map[string]map[string]bool{}
	scanner := bufio.NewScanner(z)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 {
			continue
		}
		prefix, err := netip.ParsePrefix(fields[1])
		if err != nil || !prefix.Addr().Is6() || prefix.Addr().Is4In6() {
			continue
		}
		asn := strings.TrimPrefix(strings.ToUpper(fields[0]), "AS")
		if _, err := strconv.ParseUint(asn, 10, 32); err != nil || asn == "0" {
			continue
		}
		prefix = prefix.Masked()
		if byPrefix[prefix.String()] == nil {
			byPrefix[prefix.String()] = map[string]bool{}
		}
		byPrefix[prefix.String()][asn] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	var out []Record
	for value, origins := range byPrefix {
		prefix := netip.MustParsePrefix(value)
		record := Record{Prefix: prefix}
		for asn := range origins {
			record.Origins = append(record.Origins, asn)
		}
		sort.Strings(record.Origins)
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool {
		if c := out[i].Prefix.Addr().Compare(out[j].Prefix.Addr()); c != 0 {
			return c < 0
		}
		return out[i].Prefix.Bits() < out[j].Prefix.Bits()
	})
	if len(out) == 0 {
		return nil, fmt.Errorf("no IPv6 BGP prefixes")
	}
	return out, nil
}
