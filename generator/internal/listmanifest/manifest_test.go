package listmanifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateIsDeterministic(t *testing.T) {
	root := t.TempDir()
	for _, rel := range expectedPaths() {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		line := "2001:db8::/32\n"
		if strings.HasPrefix(rel, "ipv4/") {
			line = "192.0.2.0/24\n"
		}
		if err := os.WriteFile(path, []byte(line), 0644); err != nil {
			t.Fatal(err)
		}
	}

	changed, err := Generate(root)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("first generation should write the manifest")
	}
	first, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}

	changed, err = Generate(root)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("identical lists must not rewrite the manifest")
	}
	second, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("manifest bytes changed without a list change")
	}

	var manifest Manifest
	if err := json.Unmarshal(first, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != SchemaVersion || len(manifest.Files) != 70 {
		t.Fatalf("unexpected manifest: schema=%d files=%d", manifest.SchemaVersion, len(manifest.Files))
	}
}

func TestInspectRejectsOverlap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overlap.txt")
	if err := os.WriteFile(path, []byte("192.0.2.0/24\n192.0.2.0/25\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := inspect(path, true); err == nil {
		t.Fatal("expected overlapping CIDRs to be rejected")
	}
}
