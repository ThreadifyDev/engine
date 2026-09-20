package service

import "testing"

func TestHighestViolationSeverity(t *testing.T) {
	for _, tc := range []struct {
		name       string
		severities []string
		want       string
	}{
		{"combined critical approvals", []string{"critical", "critical"}, "critical"},
		{"critical before warning", []string{"critical", "warning"}, "critical"},
		{"critical after warning", []string{"warning", "critical"}, "critical"},
		{"major and minor", []string{"minor", "major"}, "major"},
		{"unclassified", []string{""}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var violations []map[string]interface{}
			for _, severity := range tc.severities {
				violations = append(violations, map[string]interface{}{"severity": severity})
			}
			if got := highestViolationSeverity(violations); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
