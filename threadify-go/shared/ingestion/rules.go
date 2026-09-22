// Package ingestion defines span-name rules shared by management and OTLP ingestion.
package ingestion

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var ErrConflict = errors.New("ingestion rules changed; reload before saving")

const MaxFilters = 100
const MaxPatternBytes = 256

type Settings struct {
	Filters        []string   `json:"filters"`
	Revision       string     `json:"revision"`
	UpdatedAt      *time.Time `json:"updated_at,omitempty"`
	EvaluatedSpans int64      `json:"evaluated_spans"`
	DroppedSpans   int64      `json:"dropped_spans"`
}

type Store interface {
	Load(context.Context, string) (Settings, error)
	Save(context.Context, string, string, []string) (Settings, error)
	Record(context.Context, string, int, int) error
}

// Normalize rejects empty names and unsupported interior wildcards.
func Normalize(filters []string) ([]string, error) {
	if len(filters) > MaxFilters {
		return nil, fmt.Errorf("at most %d filters are allowed", MaxFilters)
	}
	result := make([]string, 0, len(filters))
	seen := map[string]bool{}
	for _, raw := range filters {
		p := strings.TrimSpace(raw)
		if p == "" || !utf8.ValidString(p) || len(p) > MaxPatternBytes || strings.IndexFunc(p, unicode.IsControl) >= 0 {
			return nil, fmt.Errorf("filters must be nonblank names of at most %d UTF-8 bytes without control characters", MaxPatternBytes)
		}
		if strings.Contains(strings.TrimSuffix(p, "*"), "*") {
			return nil, errors.New("only one trailing * wildcard is supported")
		}
		if !seen[p] {
			result = append(result, p)
			seen[p] = true
		}
	}
	return result, nil
}

// Match uses original, case-sensitive span names, matching the Threadify SDK exporters.
func Match(filters []string, name string) string {
	for _, p := range filters {
		if strings.HasSuffix(p, "*") {
			if strings.HasPrefix(name, strings.TrimSuffix(p, "*")) {
				return p
			}
		} else if name == p {
			return p
		}
	}
	return ""
}

type PreviewSpan struct {
	Name    string `json:"name"`
	Drop    bool   `json:"drop"`
	Pattern string `json:"pattern,omitempty"`
}
type PreviewResult struct {
	Spans   []PreviewSpan `json:"spans"`
	Dropped int           `json:"dropped"`
	Kept    int           `json:"kept"`
}

func Preview(filters, names []string) (PreviewResult, error) {
	normalized, err := Normalize(filters)
	if err != nil {
		return PreviewResult{}, err
	}
	if len(names) > 100 {
		return PreviewResult{}, errors.New("preview accepts at most 100 span names")
	}
	result := PreviewResult{Spans: make([]PreviewSpan, 0, len(names))}
	for _, name := range names {
		if len(name) > 4096 || !utf8.ValidString(name) {
			return PreviewResult{}, errors.New("preview span names must be valid UTF-8 and at most 4096 bytes")
		}
		pattern := Match(normalized, name)
		result.Spans = append(result.Spans, PreviewSpan{Name: name, Drop: pattern != "", Pattern: pattern})
		if pattern != "" {
			result.Dropped++
		} else {
			result.Kept++
		}
	}
	return result, nil
}
