package apnicinetnum

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestParseFiltersDuringParsingAndPreservesFullRecordCount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inetnum.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := gzip.NewWriter(f)
	data := "inetnum: 10.0.0.0 - 10.255.255.255\nnetname: RELEVANT\n\n" +
		"inetnum: 192.0.2.0 - 192.0.2.255\nnetname: IRRELEVANT\n\n" +
		"inetnum: 192.0.2.0 - 192.0.2.255\ndescr: duplicate range\n\n"
	if _, err := z.Write([]byte(data)); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	records, recordCount, err := Parse(path, func(lo, hi uint32) bool {
		return lo <= 0x0affffff && hi >= 0x0a000000
	})
	if err != nil {
		t.Fatal(err)
	}
	if recordCount != 2 || len(records) != 1 || records[0].Netnames[0] != "RELEVANT" {
		t.Fatalf("recordCount=%d records=%#v", recordCount, records)
	}
}
