package contractcontent

import (
	"fmt"
	"regexp"
	"sync"
)

const maxPatternBytes = 4096
const patternCacheSize = 128

// Bound retained patterns across contract versions and tenants. Compiled Go
// regexes are safe for concurrent use and do not use backtracking matching.
var patterns = struct {
	sync.RWMutex
	entries map[string]*regexp.Regexp
	order   []string
}{entries: make(map[string]*regexp.Regexp)}

func compilePattern(source string) (*regexp.Regexp, error) {
	if len(source) == 0 || len(source) > maxPatternBytes {
		return nil, fmt.Errorf("pattern must contain 1 to %d bytes", maxPatternBytes)
	}
	patterns.RLock()
	compiled := patterns.entries[source]
	patterns.RUnlock()
	if compiled != nil {
		return compiled, nil
	}
	compiled, err := regexp.Compile(source)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %w", err)
	}
	patterns.Lock()
	defer patterns.Unlock()
	if existing := patterns.entries[source]; existing != nil {
		return existing, nil
	}
	if len(patterns.order) == patternCacheSize {
		delete(patterns.entries, patterns.order[0])
		patterns.order = patterns.order[1:]
	}
	patterns.order = append(patterns.order, source)
	patterns.entries[source] = compiled
	return compiled, nil
}
