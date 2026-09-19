package contractcontent

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestRegexContent(t *testing.T) {
	for _, tc := range []struct {
		name, pattern, value string
		missing, want        bool
	}{
		{"match", `^TRK-[0-9]{8}$`, "TRK-12345678", false, true},
		{"mismatch", `^TRK-[0-9]{8}$`, "TRK-123", false, false},
		{"anchored", `^TRK-[0-9]{8}$`, "xTRK-12345678x", false, false},
		{"substring", `TRK-[0-9]{8}`, "xTRK-12345678x", false, true},
		{"case sensitive", `^TRK`, "trk-123", false, false},
		{"case flag", `(?i)^TRK`, "trk-123", false, true},
		{"unicode", `^\p{L}+$`, "Élodie", false, true},
		{"escaped", `^\d+\.\d{2}$`, "12.50", false, true},
		{"empty allowed explicitly", `^$`, "", false, true},
		{"empty rejected", `.+`, "", false, false},
		{"missing", `.*`, "", true, false},
		{"newline", `^.+$`, "one\ntwo", false, false},
		{"dotall", `(?s)^.+$`, "one\ntwo", false, true},
		{"nested repetition", `^(a+)+$`, strings.Repeat("a", 100000) + "!", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			content := map[string]string{}
			if !tc.missing {
				content["v"] = tc.value
			}
			err := Check([]Rule{{Field: "v", Operator: "matches", Value: tc.pattern}}, content)
			if (err == nil) != tc.want {
				t.Fatalf("want allowed=%v, got %v", tc.want, err)
			}
		})
	}
}

func TestMalformedRegexRulesFailClosed(t *testing.T) {
	for _, rule := range []Rule{
		{Field: "v", Operator: "matches", Value: "["},
		{Field: "v", Operator: "matches", Value: ""},
		{Field: "v", Operator: "matches", Value: `(?=a)a`},
		{Field: "v", Operator: "matches", Value: `(a)\1`},
		{Field: "v", Operator: "matches", Value: strings.Repeat("a", maxPatternBytes+1)},
		{Field: "v", Operator: "matches", Value: ".*", Values: []string{"a"}},
		{Field: "v", Operator: "matches", Value: ".*", Reference: &Reference{Step: "before", Field: "v"}},
	} {
		if Validate([]Rule{rule}) == nil {
			t.Fatal("accepted malformed regex rule")
		}
		if Check([]Rule{rule}, map[string]string{"v": "anything"}) == nil {
			t.Fatal("malformed persisted rule failed open")
		}
	}
}

func TestRegexCacheConcurrentEviction(t *testing.T) {
	var workers sync.WaitGroup
	for w := 0; w < 4; w++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := 0; i < patternCacheSize*2; i++ {
				value := fmt.Sprintf("value%d", i)
				if err := Check([]Rule{{Field: "v", Operator: "matches", Value: "^" + value + "$"}}, map[string]string{"v": value}); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	workers.Wait()
	patterns.RLock()
	defer patterns.RUnlock()
	if len(patterns.entries) > patternCacheSize || len(patterns.order) > patternCacheSize {
		t.Fatal("regex cache exceeded its bound")
	}
}

func BenchmarkRegexCheck(b *testing.B) {
	rules := []Rule{{Field: "tracking", Operator: "matches", Value: `^TRK-[0-9]{8}$`}}
	content := map[string]string{"tracking": "TRK-12345678"}
	if err := Check(rules, content); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Check(rules, content); err != nil {
			b.Fatal(err)
		}
	}
}
