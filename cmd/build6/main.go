package main

import (
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/closur3/cn-eyeball-prefixes/internal/apnic6"
	"github.com/closur3/cn-eyeball-prefixes/internal/riswhois6"
)

const (
	telecomBlock      = "240e::/18"
	fixedDescription  = "Chinatelecom IPv6 address for fixed broadband"
	mobileDescription = "Chinatelecom IPv6 address for mobile"
)

type asMeta struct {
	Country     string
	Description string
}

type sourceMeta struct {
	Source string `json:"source"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type outputMeta struct {
	Path                  string         `json:"path"`
	PrefixCount           int            `json:"prefix_count"`
	AdmissionDescriptions []string       `json:"admission_descriptions"`
	AdmissionMatches      map[string]int `json:"admission_matches"`
}

type manifest struct {
	GeneratedAt string                `json:"generated_at"`
	Sources     map[string]sourceMeta `json:"sources"`
	Boundary    string                `json:"telecom_boundary"`
	OutputUnit  string                `json:"output_unit"`
	Outputs     map[string]outputMeta `json:"outputs"`
	Rejected    map[string]int        `json:"rejected_bgp_prefixes"`
}

func main() {
	risPath := flag.String("ris", "", "RIPE RISWhois IPv6 gzip")
	iptoasnPath := flag.String("iptoasn", "", "IPtoASN IPv6 TSV gzip")
	inet6numPath := flag.String("inet6num", "", "APNIC inet6num gzip")
	outputPath := flag.String("output", "data/ipv6/operators/chinatelecom.txt", "China Telecom access prefix output")
	manifestPath := flag.String("manifest", "data/ipv6/manifest.json", "manifest path")
	flag.Parse()
	if *risPath == "" || *iptoasnPath == "" || *inet6numPath == "" {
		panic("--ris, --iptoasn, and --inet6num are required")
	}

	boundary := netip.MustParsePrefix(telecomBlock)
	metadata, err := readASNMetadata(*iptoasnPath)
	must(err)
	bgpRecords, err := riswhois6.Parse(*risPath)
	must(err)
	registrations, err := apnic6.Parse(*inet6numPath, boundary)
	must(err)
	segments := apnic6.ResolveMostSpecific(registrations)

	var admitted []netip.Prefix
	admissionMatches := map[string]int{"fixed_broadband": 0, "mobile": 0}
	rejected := map[string]int{}
	for _, record := range bgpRecords {
		if !inside(record.Prefix, boundary) {
			continue
		}
		if !allTelecomOrigins(record.Origins, metadata) {
			rejected["non_telecom_origin"]++
			continue
		}
		purpose, reason := classifyPrefix(record.Prefix, segments)
		switch purpose {
		case "fixed":
			admitted = append(admitted, record.Prefix)
			admissionMatches["fixed_broadband"]++
		case "mobile":
			admitted = append(admitted, record.Prefix)
			admissionMatches["mobile"]++
		default:
			rejected[reason]++
		}
	}

	sort.Slice(admitted, func(i, j int) bool {
		if c := admitted[i].Addr().Compare(admitted[j].Addr()); c != 0 {
			return c < 0
		}
		return admitted[i].Bits() < admitted[j].Bits()
	})
	must(writePrefixes(*outputPath, admitted))
	sources := map[string]sourceMeta{}
	for name, item := range map[string]struct{ path, source string }{
		"riswhois_ipv6": {*risPath, "https://www.ris.ripe.net/dumps/riswhoisdump.IPv6.gz"},
		"iptoasn_ipv6":  {*iptoasnPath, "https://iptoasn.com/data/ip2asn-v6.tsv.gz"},
		"apnic_inet6num": {*inet6numPath, "https://ftp.apnic.net/apnic/whois/apnic.db.inet6num.gz"},
	} {
		meta, err := fileMetadata(item.path)
		must(err)
		meta.Source = item.source
		sources[name] = meta
	}
	result := manifest{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Sources: sources,
		Boundary: telecomBlock,
		OutputUnit: "exact_current_bgp_prefix",
		Outputs: map[string]outputMeta{
			"chinatelecom": {
				Path:                  filepath.ToSlash(*outputPath),
				PrefixCount:           len(admitted),
				AdmissionDescriptions: []string{fixedDescription, mobileDescription},
				AdmissionMatches:      admissionMatches,
			},
		},
		Rejected: rejected,
	}
	must(writeJSON(*manifestPath, result))
}

func classifyPrefix(prefix netip.Prefix, segments []apnic6.Segment) (string, string) {
	lo, hi := prefix.Masked().Addr(), lastAddress(prefix)
	i := sort.Search(len(segments), func(i int) bool { return segments[i].Hi.Compare(lo) >= 0 })
	cursor := lo
	purpose := ""
	for ; i < len(segments) && segments[i].Lo.Compare(hi) <= 0; i++ {
		segment := segments[i]
		if segment.Lo.Compare(cursor) > 0 {
			return "", "registration_gap"
		}
		value := registrationPurpose(segment.Record)
		if value == "" {
			return "", "description_not_admitted"
		}
		if purpose != "" && purpose != value {
			return "", "mixed_access_purpose"
		}
		purpose = value
		if segment.Hi.Compare(hi) >= 0 {
			return purpose, ""
		}
		cursor = segment.Hi.Next()
	}
	return "", "registration_gap"
}

func registrationPurpose(record apnic6.Record) string {
	for _, description := range record.Descriptions {
		switch {
		case strings.EqualFold(strings.TrimSpace(description), fixedDescription):
			return "fixed"
		case strings.EqualFold(strings.TrimSpace(description), mobileDescription):
			return "mobile"
		}
	}
	return ""
}

func allTelecomOrigins(origins []string, metadata map[string]asMeta) bool {
	if len(origins) == 0 {
		return false
	}
	for _, asn := range origins {
		meta, ok := metadata[asn]
		if !ok || !strings.EqualFold(meta.Country, "CN") || !isTelecom(asn, meta.Description) {
			return false
		}
	}
	return true
}

func isTelecom(asn, description string) bool {
	if asn == "4847" {
		return true
	}
	value := strings.ToLower(description)
	return strings.Contains(value, "china telecom") || strings.Contains(value, "chinatelecom") || strings.Contains(value, "chinanet")
}

func readASNMetadata(path string) (map[string]asMeta, error) {
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
	type choice struct {
		meta  asMeta
		count int
	}
	choices := map[string]map[string]*choice{}
	scanner := bufio.NewScanner(z)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) < 5 || fields[2] == "0" {
			continue
		}
		key := fields[3] + "\x00" + fields[4]
		if choices[fields[2]] == nil {
			choices[fields[2]] = map[string]*choice{}
		}
		entry := choices[fields[2]][key]
		if entry == nil {
			entry = &choice{meta: asMeta{Country: fields[3], Description: fields[4]}}
			choices[fields[2]][key] = entry
		}
		entry.count++
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	out := map[string]asMeta{}
	for asn, variants := range choices {
		best := &choice{}
		for _, candidate := range variants {
			if candidate.count > best.count || (candidate.count == best.count && candidate.meta.Description < best.meta.Description) {
				best = candidate
			}
		}
		out[asn] = best.meta
	}
	return out, nil
}

func inside(prefix, boundary netip.Prefix) bool {
	return boundary.Contains(prefix.Masked().Addr()) && boundary.Contains(lastAddress(prefix))
}

func lastAddress(prefix netip.Prefix) netip.Addr {
	b := prefix.Masked().Addr().As16()
	for bit := prefix.Bits(); bit < 128; bit++ {
		b[bit/8] |= 1 << uint(7-bit%8)
	}
	return netip.AddrFrom16(b)
}

func writePrefixes(path string, prefixes []netip.Prefix) error {
	if len(prefixes) == 0 {
		return fmt.Errorf("refusing to write empty prefix list: %s", path)
	}
	var b strings.Builder
	for _, prefix := range prefixes {
		fmt.Fprintln(&b, prefix)
	}
	return writeFile(path, []byte(b.String()))
}

func fileMetadata(path string) (sourceMeta, error) {
	f, err := os.Open(path)
	if err != nil {
		return sourceMeta{}, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return sourceMeta{}, err
	}
	return sourceMeta{Bytes: n, SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}

func writeJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(path, append(b, '\n'))
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
