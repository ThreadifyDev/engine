// Package ingestion defines span-name rules shared by management and OTLP ingestion.
package ingestion

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var ErrConflict = errors.New("ingestion rules changed; reload before saving")

const MaxFilters = 100
const MaxPatternBytes = 256
const ModeInclude = "include"
const ModeExcludeLegacy = "exclude_legacy"

type Settings struct {
	Exclude        []string   `json:"exclude"`
	Filters        []string   `json:"filters"`
	Mode           string     `json:"mode"`
	Revision       string     `json:"revision"`
	UpdatedAt      *time.Time `json:"updated_at,omitempty"`
	EvaluatedSpans int64      `json:"evaluated_spans"`
	DroppedSpans   int64      `json:"dropped_spans"`
}

type Store interface {
	Load(context.Context, string) (Settings, error)
	Save(context.Context, string, string, []string, []string) (Settings, error)
	Record(context.Context, string, int, int) error
}

// Normalize validates exact, trailing-wildcard, and explicit regex patterns.
func Normalize(filters []string) ([]string, error) {
	patterns, err := compilePatterns(filters)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(patterns))
	for _, p := range patterns {
		result = append(result, p.source)
	}
	return result, nil
}

type pattern struct {
	source     string
	expression *regexp.Regexp
}

func compilePatterns(filters []string) ([]pattern, error) {
	if len(filters) > MaxFilters {
		return nil, fmt.Errorf("at most %d filters are allowed", MaxFilters)
	}
	result := make([]pattern, 0, len(filters))
	seen := map[string]bool{}
	for i, raw := range filters {
		p := strings.TrimSpace(raw)
		if p == "" || !utf8.ValidString(p) || len(p) > MaxPatternBytes || strings.IndexFunc(p, unicode.IsControl) >= 0 {
			return nil, fmt.Errorf("pattern %d must be nonblank and at most %d UTF-8 bytes without control characters", i+1, MaxPatternBytes)
		}
		if seen[p] {
			continue
		}
		compiled := pattern{source: p}
		if expression, ok := strings.CutPrefix(p, "regex:"); ok {
			if expression == "" {
				return nil, fmt.Errorf("pattern %d: regex expression is empty", i+1)
			}
			var err error
			compiled.expression, err = regexp.Compile(expression)
			if err != nil {
				return nil, fmt.Errorf("pattern %d: invalid regex: %w", i+1, err)
			}
		} else if strings.Contains(strings.TrimSuffix(p, "*"), "*") {
			return nil, errors.New("only one trailing * wildcard is supported; use regex: for regular expressions")
		}
		result = append(result, compiled)
		seen[p] = true
	}
	return result, nil
}

func match(patterns []pattern, name string) string {
	for _, p := range patterns {
		if p.expression != nil {
			if p.expression.MatchString(name) {
				return p.source
			}
		} else if strings.HasSuffix(p.source, "*") {
			if strings.HasPrefix(name, strings.TrimSuffix(p.source, "*")) {
				return p.source
			}
		} else if name == p.source {
			return p.source
		}
	}
	return ""
}

// Match is a convenience for individual names. Batch callers use Compile.
// Plain names remain case-sensitive; regex: patterns can opt into flags like (?i).
func Match(filters []string, name string) string {
	patterns, err := compilePatterns(filters)
	if err != nil {
		return ""
	}
	return match(patterns, name)
}

// Matcher holds compiled expressions for reuse throughout one ingestion batch.
type Matcher struct {
	mode             string
	filters, exclude []pattern
}

func Compile(settings Settings) (*Matcher, error) {
	filters, err := compilePatterns(settings.Filters)
	if err != nil {
		return nil, fmt.Errorf("keep spans: %w", err)
	}
	exclude, err := compilePatterns(settings.Exclude)
	if err != nil {
		return nil, fmt.Errorf("drop spans: %w", err)
	}
	return &Matcher{mode: settings.Mode, filters: filters, exclude: exclude}, nil
}

// ShouldDrop keeps saved exclusion policies working until an administrator
// explicitly replaces them with an inclusion policy.
func ShouldDrop(mode string, filters []string, name string) bool {
	matched := Match(filters, name) != ""
	return mode == ModeExcludeLegacy && matched || mode != ModeExcludeLegacy && !matched
}

type PreviewSpan struct {
	DropPattern string `json:"drop_pattern,omitempty"`
	Name        string `json:"name"`
	Drop        bool   `json:"drop"`
	Pattern     string `json:"pattern,omitempty"`
}
type PreviewResult struct {
	Spans   []PreviewSpan `json:"spans"`
	Dropped int           `json:"dropped"`
	Kept    int           `json:"kept"`
}

// Evaluate is a convenience for validated settings and a single span.
// Invalid settings fail closed; ingestion and preview use Compile to return errors.
func Evaluate(settings Settings, name string) PreviewSpan {
	matcher, err := Compile(settings)
	if err != nil {
		return PreviewSpan{Name: name, Drop: true}
	}
	return matcher.Evaluate(name)
}

func (m *Matcher) Evaluate(name string) PreviewSpan {
	result := PreviewSpan{Name: name, Pattern: match(m.filters, name)}
	result.Drop = m.mode == ModeExcludeLegacy && result.Pattern != "" || m.mode != ModeExcludeLegacy && result.Pattern == ""
	if result.Drop {
		return result
	}
	if result.DropPattern = match(m.exclude, name); result.DropPattern != "" {
		result.Drop = true
	}
	return result
}

func Preview(filters, names []string) (PreviewResult, error) {
	return PreviewRules(filters, nil, names)
}

func PreviewRules(filters, exclude, names []string) (PreviewResult, error) {
	matcher, err := Compile(Settings{Mode: ModeInclude, Filters: filters, Exclude: exclude})
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
		decision := matcher.Evaluate(name)
		result.Spans = append(result.Spans, decision)
		if decision.Drop {
			result.Dropped++
		} else {
			result.Kept++
		}
	}
	return result, nil
}
