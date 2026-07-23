package listmanifest

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const SchemaVersion = 2

var operators = []string{"chinatelecom", "chinamobile", "chinaunicom"}

var provinces = []string{
	"anhui",
	"beijing",
	"chongqing",
	"fujian",
	"gansu",
	"guangdong",
	"guangxi",
	"guizhou",
	"hainan",
	"hebei",
	"heilongjiang",
	"henan",
	"hubei",
	"hunan",
	"jiangsu",
	"jiangxi",
	"jilin",
	"liaoning",
	"neimenggu",
	"ningxia",
	"qinghai",
	"shaanxi",
	"shandong",
	"shanghai",
	"shanxi",
	"sichuan",
	"tianjin",
	"xinjiang",
	"xizang",
	"yunnan",
	"zhejiang",
}

type File struct {
	PrefixCount int    `json:"prefix_count"`
	SHA256      string `json:"sha256"`
}

type Manifest struct {
	SchemaVersion int             `json:"schema_version"`
	ContentID     string          `json:"content_id"`
	Files         map[string]File `json:"files"`
}

// Generate validates the complete public list contract and writes a deterministic
// manifest. It returns false without touching the file when its bytes are already
// current.
func Generate(root string) (bool, error) {
	paths := expectedPaths()
	if err := rejectUnexpectedLists(root, paths); err != nil {
		return false, err
	}

	files := make(map[string]File, len(paths))
	for _, rel := range paths {
		meta, err := inspect(filepath.Join(root, filepath.FromSlash(rel)), strings.HasPrefix(rel, "ipv4/"))
		if err != nil {
			return false, fmt.Errorf("%s: %w", rel, err)
		}
		files[rel] = meta
	}

	manifest := Manifest{
		SchemaVersion: SchemaVersion,
		ContentID:     contentID(paths, files),
		Files:         files,
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return false, err
	}
	data = append(data, '\n')

	path := filepath.Join(root, "manifest.json")
	current, err := os.ReadFile(path)
	if err == nil && bytes.Equal(current, data) {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return false, err
	}
	return true, nil
}

func expectedPaths() []string {
	var paths []string
	for _, family := range []string{"ipv4", "ipv6"} {
		paths = append(paths, family+"/cn.txt")
		for _, operator := range operators {
			paths = append(paths, family+"/"+operator+".txt")
		}
		for _, province := range provinces {
			paths = append(paths, family+"/provinces/"+province+".txt")
		}
	}
	sort.Strings(paths)
	return paths
}

func rejectUnexpectedLists(root string, expected []string) error {
	want := make(map[string]bool, len(expected))
	for _, rel := range expected {
		want[rel] = true
	}
	var unexpected []string
	for _, family := range []string{"ipv4", "ipv6"} {
		base := filepath.Join(root, family)
		err := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".txt") {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if !want[rel] {
				unexpected = append(unexpected, rel)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	if len(unexpected) != 0 {
		sort.Strings(unexpected)
		return fmt.Errorf("unexpected public list files: %s", strings.Join(unexpected, ", "))
	}
	return nil
}

func inspect(path string, ipv4 bool) (File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	sum := sha256.Sum256(data)

	scanner := bufio.NewScanner(bytes.NewReader(data))
	var previous netip.Prefix
	count := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			return File{}, fmt.Errorf("blank line at line %d", count+1)
		}
		prefix, err := netip.ParsePrefix(line)
		if err != nil {
			return File{}, fmt.Errorf("invalid CIDR at line %d: %w", count+1, err)
		}
		if prefix != prefix.Masked() {
			return File{}, fmt.Errorf("non-canonical CIDR at line %d: %s", count+1, line)
		}
		if line != prefix.String() {
			return File{}, fmt.Errorf("non-canonical CIDR text at line %d: use %s", count+1, prefix)
		}
		if prefix.Addr().Is4() != ipv4 {
			return File{}, fmt.Errorf("wrong address family at line %d: %s", count+1, line)
		}
		if count != 0 {
			if compare(previous, prefix) >= 0 {
				return File{}, fmt.Errorf("CIDRs are not strictly sorted at line %d", count+1)
			}
			if previous.Contains(prefix.Addr()) {
				return File{}, fmt.Errorf("CIDRs overlap at line %d: %s contains %s", count+1, previous, prefix)
			}
		}
		previous = prefix
		count++
	}
	if err := scanner.Err(); err != nil {
		return File{}, err
	}
	return File{PrefixCount: count, SHA256: hex.EncodeToString(sum[:])}, nil
}

func compare(a, b netip.Prefix) int {
	if cmp := a.Addr().Compare(b.Addr()); cmp != 0 {
		return cmp
	}
	switch {
	case a.Bits() < b.Bits():
		return -1
	case a.Bits() > b.Bits():
		return 1
	default:
		return 0
	}
}

func contentID(paths []string, files map[string]File) string {
	hash := sha256.New()
	fmt.Fprintf(hash, "schema=%d\n", SchemaVersion)
	for _, path := range paths {
		meta := files[path]
		fmt.Fprintf(hash, "%s\x00%d\x00%s\n", path, meta.PrefixCount, meta.SHA256)
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}
