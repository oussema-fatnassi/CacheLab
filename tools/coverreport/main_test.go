package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestReportWeightedCoverageAndWarning(t *testing.T) {
	for _, tc := range []struct {
		name, blocks, percentage string
		warning                  bool
	}{
		{"weighted", "a.go:1.1,2.1 1 7\nb.go:1.1,2.1 9 0\n", "10.00%", true},
		{"at threshold", "a.go:1.1,2.1 4 1\na.go:3.1,4.1 1 0\n", "80.00%", false},
		{"below despite rounding", "a.go:1.1,2.1 79999 1\na.go:3.1,4.1 20001 0\n", "80.00%", true},
		{"all covered", "a.go:1.1,2.1 3 2\n", "100.00%", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := report(strings.NewReader("mode: atomic\n"+tc.blocks), &output); err != nil {
				t.Fatal(err) // Low coverage must not fail the command.
			}
			if !strings.Contains(output.String(), tc.percentage) || strings.Contains(output.String(), "WARNING") != tc.warning {
				t.Fatalf("unexpected report: %s", output.String())
			}
		})
	}
}

func TestReportRejectsInvalidOrEmptyProfile(t *testing.T) {
	for _, profile := range []string{"", "mode: atomic\n", "mode: atomic\nbroken\n", "mode: atomic\na.go:1.1,2.1 x 1\n"} {
		var output bytes.Buffer
		if err := report(strings.NewReader(profile), &output); err == nil {
			t.Fatalf("accepted invalid profile %q", profile)
		}
	}
}
