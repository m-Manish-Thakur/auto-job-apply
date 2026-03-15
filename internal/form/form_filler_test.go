package form

import (
	"testing"
)

func TestResolveFieldKey(t *testing.T) {
	cases := []struct {
		label    string
		expected string
	}{
		{"Full Name", "full name"},
		{"Email Address", "email"},
		{"Mobile Number", "mobile"},
		{"Phone", "mobile"},
		{"Years of Experience", "year"},
		{"Total Experience", "year"},
		{"Current Location", "location"},
		{"City", "location"},
		{"Key Skills", "skill"},
		{"UnknownField", ""},
	}

	for _, tc := range cases {
		got := ResolveFieldKey(tc.label)
		if got != tc.expected {
			t.Errorf("ResolveFieldKey(%q) = %q; want %q", tc.label, got, tc.expected)
		}
	}
}
