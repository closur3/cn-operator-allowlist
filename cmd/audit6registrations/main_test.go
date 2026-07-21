package main

import (
	"testing"

	"github.com/closur3/cn-eyeball-prefixes/internal/apnic6"
)

func TestIsPortableStatus(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{"ALLOCATED PORTABLE", true},
		{"ASSIGNED PORTABLE", true},
		{"ALLOCATED NON-PORTABLE", false},
		{"ASSIGNED NON-PORTABLE", false},
		{"", false},
	}
	for _, test := range tests {
		if got := isPortableStatus(test.status); got != test.want {
			t.Fatalf("isPortableStatus(%q) = %v, want %v", test.status, got, test.want)
		}
	}
}

func TestExplicitUserPurposeWhitelist(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"Chinatelecom IPv6 address for fixed broadband", true},
		{"Chinatelecom IPv6 address for mobile", true},
		{"BeiJing-Telecom-UserAddress-lowguaranteed", true},
		{"China Telecom Jiangsu province network for customer", false},
		{"Including users who access to Internet through Chinatelecom's networks", false},
		{"China Telecom Internet Service Provider", false},
	}
	for _, test := range tests {
		record := apnic6.InetRecord{Descriptions: []string{test.text}}
		if got := isExplicitUserPurpose(record); got != test.want {
			t.Fatalf("isExplicitUserPurpose(%q) = %v, want %v", test.text, got, test.want)
		}
	}
}
