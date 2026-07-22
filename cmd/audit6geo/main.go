package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ip2location/ip2location-go/v9"
)

const (
	dataSourceURL = "https://github.com/renfei/ip2location/releases"
	dataLicense   = "CC BY-SA 4.0"
)

type inputManifest struct {
	RegistryAdmission struct {
		MatchedPrefixes map[string][]string `json:"matched_inet6num_prefixes"`
	} `json:"registry_admission"`
	CuratedAdmission map[string]map[string][]string `json:"curated_admission"`
}

type admissionParent struct {
	Prefix  netip.Prefix
	Purpose string
}

type geoRecord struct {
	Country string `json:"country"`
	Region  string `json:"region"`
	City    string `json:"city,omitempty"`
}

type unresolvedRecord struct {
	Operator string    `json:"operator"`
	Purpose  string    `json:"purpose"`
	Prefix   string    `json:"prefix"`
	Reason   string    `json:"reason"`
	First    geoRecord `json:"first"`
	Last     geoRecord `json:"last"`
}

type provinceResult struct {
	Code              string   `json:"code"`
	Name              string   `json:"name"`
	RawRegions        []string `json:"raw_regions"`
	PrefixCount       int      `json:"prefix_count"`
	Slash32Equivalent string   `json:"slash32_equivalent"`
	Path              string   `json:"path"`
	Prefixes          []string `json:"prefixes"`
}

type sourceResult struct {
	Repository      string `json:"repository"`
	ReleaseTag      string `json:"release_tag,omitempty"`
	Asset           string `json:"asset"`
	Bytes           int64  `json:"bytes"`
	SHA256          string `json:"sha256"`
	DatabaseVersion string `json:"database_version"`
	PackageVersion  string `json:"package_version"`
	DataLicense     string `json:"data_license"`
}

type auditResult struct {
	GeneratedAt string                                        `json:"generated_at"`
	Status      string                                        `json:"status"`
	Methodology map[string]string                             `json:"methodology"`
	Source      sourceResult                                  `json:"source"`
	Operators   map[string]map[string][]provinceResult         `json:"operators"`
	Unresolved  []unresolvedRecord                            `json:"unresolved"`
}

type province struct {
	Code string
	Name string
}

var provinces = map[string]province{
	"anhui": {"CN-AH", "安徽"}, "beijing": {"CN-BJ", "北京"},
	"chongqing": {"CN-CQ", "重庆"}, "fujian": {"CN-FJ", "福建"},
	"gansu": {"CN-GS", "甘肃"}, "guangdong": {"CN-GD", "广东"},
	"guangxi": {"CN-GX", "广西"}, "guangxizhuang": {"CN-GX", "广西"}, "guangxizhuangzu": {"CN-GX", "广西"},
	"guizhou": {"CN-GZ", "贵州"}, "hainan": {"CN-HI", "海南"},
	"hebei": {"CN-HE", "河北"}, "heilongjiang": {"CN-HL", "黑龙江"},
	"henan": {"CN-HA", "河南"}, "hubei": {"CN-HB", "湖北"},
	"hunan": {"CN-HN", "湖南"}, "innermongolia": {"CN-NM", "内蒙古"},
	"neimenggu": {"CN-NM", "内蒙古"}, "jiangsu": {"CN-JS", "江苏"},
	"jiangxi": {"CN-JX", "江西"}, "jilin": {"CN-JL", "吉林"},
	"liaoning": {"CN-LN", "辽宁"}, "ningxia": {"CN-NX", "宁夏"},
	"ningxiahui": {"CN-NX", "宁夏"}, "ningxiahuizu": {"CN-NX", "宁夏"}, "qinghai": {"CN-QH", "青海"},
	"shaanxi": {"CN-SN", "陕西"}, "shandong": {"CN-SD", "山东"},
	"shanghai": {"CN-SH", "上海"}, "shanxi": {"CN-SX", "山西"},
	"sichuan": {"CN-SC", "四川"}, "tianjin": {"CN-TJ", "天津"},
	"tibet": {"CN-XZ", "西藏"}, "xizang": {"CN-XZ", "西藏"},
	"xinjiang": {"CN-XJ", "新疆"}, "xinjianguygur": {"CN-XJ", "新疆"}, "xinjianguyghur": {"CN-XJ", "新疆"},
	"yunnan": {"CN-YN", "云南"}, "zhejiang": {"CN-ZJ", "浙江"},
}

func main() {
	dbPath := flag.String("db", "", "IP2Location DB11 IPv6 BIN file")
	manifestPath := flag.String("manifest", "data/ipv6/manifest.json", "IPv6 admission manifest")
	operatorDir := flag.String("operator-dir", "data/ipv6/operators", "operator BGP prefix directory")
	outputDir := flag.String("output-dir", "data/ipv6/provinces", "province prefix output directory")
	auditJSON := flag.String("audit-json", "data/ipv6/audits/ip2location-provinces.json", "machine-readable audit")
	auditMarkdown := flag.String("audit-markdown", "data/ipv6/audits/ip2location-provinces.md", "human-readable audit")
	releaseTag := flag.String("release-tag", "", "renfei/ip2location release tag")
	flag.Parse()
	if *dbPath == "" {
		panic("--db is required")
	}

	manifest, err := readManifest(*manifestPath)
	must(err)
	parents, err := admissionParents(manifest)
	must(err)
	db, err := ip2location.OpenDB(*dbPath)
	must(err)
	defer db.Close()

	must(resetOutputDir(*outputDir))
	grouped := map[string]map[string]map[string][]netip.Prefix{}
	rawRegions := map[string]map[string]map[string]map[string]struct{}{}
	var unresolved []unresolvedRecord
	for _, operator := range []string{"chinatelecom", "chinamobile", "chinaunicom"} {
		prefixes, err := readPrefixes(filepath.Join(*operatorDir, operator+".txt"))
		must(err)
		byPurpose := map[string][]netip.Prefix{}
		for _, prefix := range prefixes {
			purpose, ok := purposeForPrefix(prefix, parents[operator])
			if !ok {
				unresolved = append(unresolved, unresolvedRecord{Operator: operator, Prefix: prefix.String(), Reason: "no_unique_admission_parent"})
				continue
			}
			byPurpose[purpose] = append(byPurpose[purpose], prefix)
		}
		for purpose, values := range byPurpose {
			for _, unit := range values {
				first, err := lookup(db, unit.Addr())
				must(err)
				last, err := lookup(db, lastAddress(unit))
				must(err)
				p, reason := classifyProvince(first, last)
				if reason != "" {
					unresolved = append(unresolved, unresolvedRecord{Operator: operator, Purpose: purpose, Prefix: unit.String(), Reason: reason, First: first, Last: last})
					continue
				}
				ensureGroup(grouped, operator, purpose)
				grouped[operator][purpose][p.Code] = append(grouped[operator][purpose][p.Code], unit)
				ensureRawGroup(rawRegions, operator, purpose, p.Code)
				rawRegions[operator][purpose][p.Code][first.Region] = struct{}{}
			}
		}
	}

	result := auditResult{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Status:      "audit_only",
		Methodology: map[string]string{
			"input":               "current admitted BGP prefixes from data/ipv6/operators",
			"classification_unit": "each exact current BGP prefix is classified without splitting or aggregation",
			"consistency_check":   "the first and last address of every original BGP prefix must resolve to the same mainland-China province; all other results remain unresolved",
			"effect_on_admission": "none; this output is for manual validation and does not alter the formal IPv6 lists",
		},
		Source:     makeSource(*dbPath, *releaseTag, db),
		Operators:  map[string]map[string][]provinceResult{},
		Unresolved: unresolved,
	}
	for _, operator := range sortedKeys(grouped) {
		result.Operators[operator] = map[string][]provinceResult{}
		for _, purpose := range sortedKeys(grouped[operator]) {
			for _, code := range sortedKeys(grouped[operator][purpose]) {
				values := grouped[operator][purpose][code]
				sort.Slice(values, func(i, j int) bool {
					if c := values[i].Addr().Compare(values[j].Addr()); c != 0 { return c < 0 }
					return values[i].Bits() < values[j].Bits()
				})
				p := provinceByCode(code)
				path := filepath.Join(*outputDir, operator, purpose, code+".txt")
				must(writePrefixes(path, values))
				result.Operators[operator][purpose] = append(result.Operators[operator][purpose], provinceResult{
					Code: code, Name: p.Name, RawRegions: sortedSet(rawRegions[operator][purpose][code]),
					PrefixCount: len(values), Slash32Equivalent: slash32Equivalent(values),
					Path: filepath.ToSlash(path), Prefixes: prefixStrings(values),
				})
			}
		}
	}
	sort.Slice(result.Unresolved, func(i, j int) bool {
		a, b := result.Unresolved[i], result.Unresolved[j]
		if a.Operator != b.Operator { return a.Operator < b.Operator }
		if a.Purpose != b.Purpose { return a.Purpose < b.Purpose }
		return a.Prefix < b.Prefix
	})
	must(writeJSON(*auditJSON, result))
	must(writeMarkdown(*auditMarkdown, result))
}

func readManifest(path string) (inputManifest, error) {
	var out inputManifest
	b, err := os.ReadFile(path)
	if err != nil { return out, err }
	err = json.Unmarshal(b, &out)
	return out, err
}

func admissionParents(m inputManifest) (map[string][]admissionParent, error) {
	out := map[string][]admissionParent{}
	for purpose, values := range m.RegistryAdmission.MatchedPrefixes {
		for _, value := range values {
			prefix, err := netip.ParsePrefix(value)
			if err != nil { return nil, err }
			out["chinatelecom"] = append(out["chinatelecom"], admissionParent{prefix.Masked(), normalizePurpose(purpose)})
		}
	}
	for operator, purposes := range m.CuratedAdmission {
		for purpose, values := range purposes {
			for _, value := range values {
				prefix, err := netip.ParsePrefix(value)
				if err != nil { return nil, err }
				out[operator] = append(out[operator], admissionParent{prefix.Masked(), normalizePurpose(purpose)})
			}
		}
	}
	return out, nil
}

func normalizePurpose(value string) string {
	if strings.HasPrefix(value, "fixed") { return "fixed_broadband" }
	return "mobile"
}

func purposeForPrefix(prefix netip.Prefix, parents []admissionParent) (string, bool) {
	found := ""
	last := lastAddress(prefix)
	for _, parent := range parents {
		if parent.Prefix.Contains(prefix.Addr()) && parent.Prefix.Contains(last) {
			if found != "" && found != parent.Purpose { return "", false }
			found = parent.Purpose
		}
	}
	return found, found != ""
}

func readPrefixes(path string) ([]netip.Prefix, error) {
	f, err := os.Open(path)
	if err != nil { return nil, err }
	defer f.Close()
	var out []netip.Prefix
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") { continue }
		prefix, err := netip.ParsePrefix(line)
		if err != nil { return nil, fmt.Errorf("%s: %w", line, err) }
		if !prefix.Addr().Is6() { return nil, fmt.Errorf("non-IPv6 prefix %s", line) }
		out = append(out, prefix.Masked())
	}
	return out, s.Err()
}

func lookup(db *ip2location.DB, addr netip.Addr) (geoRecord, error) {
	record, err := db.Get_all(addr.String())
	if err != nil { return geoRecord{}, err }
	return geoRecord{Country: strings.TrimSpace(record.Country_short), Region: strings.TrimSpace(record.Region), City: strings.TrimSpace(record.City)}, nil
}

func classifyProvince(first, last geoRecord) (province, string) {
	if first.Country != "CN" || last.Country != "CN" { return province{}, "not_mainland_china" }
	a, okA := provinceForRegion(first.Region)
	b, okB := provinceForRegion(last.Region)
	if !okA || !okB { return province{}, "unknown_province_name" }
	if a.Code != b.Code { return province{}, "province_conflict_within_unit" }
	return a, ""
}

func provinceForRegion(value string) (province, bool) {
	key := strings.ToLower(value)
	replacer := strings.NewReplacer(" ", "", "-", "", "_", "", "province", "", "sheng", "", "autonomous", "", "region", "")
	key = replacer.Replace(key)
	p, ok := provinces[key]
	return p, ok
}

func provinceByCode(code string) province {
	for _, p := range provinces { if p.Code == code { return p } }
	return province{Code: code, Name: code}
}

func lastAddress(prefix netip.Prefix) netip.Addr {
	addr := prefix.Masked().Addr().As16()
	for bit := prefix.Bits(); bit < 128; bit++ { addr[bit/8] |= byte(1 << uint(7-bit%8)) }
	return netip.AddrFrom16(addr)
}

func slash32Equivalent(values []netip.Prefix) string {
	total := 0.0
	for _, p := range values {
		total += math.Ldexp(1, 32-p.Bits())
	}
	return fmt.Sprintf("%.9f", total)
}

func makeSource(path, tag string, db *ip2location.DB) sourceResult {
	f, err := os.Open(path); must(err); defer f.Close()
	h := sha256.New(); info, err := f.Stat(); must(err)
	_, err = io.Copy(h, f); must(err)
	return sourceResult{Repository: dataSourceURL, ReleaseTag: tag, Asset: "IP2LOCATION-LITE-DB11.IPV6.BIN", Bytes: info.Size(), SHA256: hex.EncodeToString(h.Sum(nil)), DatabaseVersion: db.DatabaseVersion(), PackageVersion: db.PackageVersion(), DataLicense: dataLicense}
}

func resetOutputDir(path string) error {
	clean := filepath.Clean(path)
	if clean == "." || filepath.Base(clean) != "provinces" { return fmt.Errorf("refusing to replace unsafe output directory %q", path) }
	if err := os.RemoveAll(clean); err != nil { return err }
	return os.MkdirAll(clean, 0o755)
}

func writePrefixes(path string, values []netip.Prefix) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { return err }
	f, err := os.Create(path); if err != nil { return err }; defer f.Close()
	w := bufio.NewWriter(f)
	for _, p := range values { if _, err := fmt.Fprintln(w, p.String()); err != nil { return err } }
	return w.Flush()
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { return err }
	b, err := json.MarshalIndent(value, "", "  "); if err != nil { return err }
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o644)
}

func writeMarkdown(path string, result auditResult) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { return err }
	var b strings.Builder
	b.WriteString("# IP2Location IPv6 省级审计\n\n")
	b.WriteString("本报告仅用于人工核对，不参与三网 IPv6 正式准入。输入为当前已准入的活跃 BGP 前缀。\n\n")
	fmt.Fprintf(&b, "- 数据：IP2Location LITE DB11 IPv6 `%s`（%s）\n", result.Source.DatabaseVersion, result.Source.DataLicense)
	fmt.Fprintf(&b, "- 发布：`%s`\n", result.Source.ReleaseTag)
	b.WriteString("- 规则：每条原始 BGP 前缀独立匹配，不拆分、不聚合；首尾地址必须落在同一省级行政区，否则进入冲突项。\n\n")
	b.WriteString("| 运营商 | 业务 | 省级代码 | 省份 | 前缀数 | /32 等价值 | 文件 |\n")
	b.WriteString("|---|---|---|---|---:|---:|---|\n")
	for _, operator := range sortedKeys(result.Operators) {
		for _, purpose := range sortedKeys(result.Operators[operator]) {
			for _, row := range result.Operators[operator][purpose] {
				fmt.Fprintf(&b, "| %s | %s | %s | %s | %d | %s | `%s` |\n", operator, purpose, row.Code, row.Name, row.PrefixCount, row.Slash32Equivalent, row.Path)
			}
		}
	}
	fmt.Fprintf(&b, "\n## 未判定与冲突\n\n共 `%d` 个分类单元。完整事实见 JSON；以下最多列出 100 项。\n\n", len(result.Unresolved))
	b.WriteString("| 运营商 | 业务 | 前缀 | 原因 | 首地址结果 | 末地址结果 |\n")
	b.WriteString("|---|---|---|---|---|---|\n")
	for i, row := range result.Unresolved {
		if i == 100 { break }
		fmt.Fprintf(&b, "| %s | %s | `%s` | %s | %s/%s/%s | %s/%s/%s |\n", row.Operator, row.Purpose, row.Prefix, row.Reason, row.First.Country, row.First.Region, row.First.City, row.Last.Country, row.Last.Region, row.Last.City)
	}
	b.WriteString("\n## 许可与边界\n\n省级结果由 IP2Location LITE 数据派生，遵循 CC BY-SA 4.0。该数据仅是待人工复核的第三方地理定位结论，不是三网省级编码的权威事实。\n")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func ensureGroup(m map[string]map[string]map[string][]netip.Prefix, operator, purpose string) {
	if m[operator] == nil { m[operator] = map[string]map[string][]netip.Prefix{} }
	if m[operator][purpose] == nil { m[operator][purpose] = map[string][]netip.Prefix{} }
}

func ensureRawGroup(m map[string]map[string]map[string]map[string]struct{}, operator, purpose, code string) {
	if m[operator] == nil { m[operator] = map[string]map[string]map[string]struct{}{} }
	if m[operator][purpose] == nil { m[operator][purpose] = map[string]map[string]struct{}{} }
	if m[operator][purpose][code] == nil { m[operator][purpose][code] = map[string]struct{}{} }
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m)); for k := range m { out = append(out, k) }; sort.Strings(out); return out
}

func sortedSet(m map[string]struct{}) []string { return sortedKeys(m) }

func prefixStrings(values []netip.Prefix) []string {
	out := make([]string, len(values)); for i, p := range values { out[i] = p.String() }; return out
}

func must(err error) { if err != nil { panic(err) } }
