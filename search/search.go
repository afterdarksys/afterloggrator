// Package search implements bounded cross-source matching and direct correlation.
package search

import (
	"afterloggrator/parsers"
	"fmt"
	"github.com/dlclark/regexp2"
	"sort"
	"strings"
	"time"
)

type Query struct {
	Regex        *regexp2.Regexp
	Contains     []string
	Fields       map[string]string
	Since, Until time.Time
	Script       *Script
}

func (q Query) InRange(e parsers.LogEntry) bool {
	if (!q.Since.IsZero() || !q.Until.IsZero()) && e.Timestamp.IsZero() {
		return false
	}
	return (q.Since.IsZero() || !e.Timestamp.Before(q.Since)) && (q.Until.IsZero() || !e.Timestamp.After(q.Until))
}
func (q Query) Match(e parsers.LogEntry) (bool, error) {
	for k, v := range q.Fields {
		if e.Field(k) != v {
			return false, nil
		}
	}
	for _, s := range q.Contains {
		if !strings.Contains(e.Line, s) {
			return false, nil
		}
	}
	if q.Regex != nil {
		ok, err := q.Regex.MatchString(e.Line)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	if q.Script != nil {
		return q.Script.Match(e)
	}
	return true, nil
}

type item struct {
	e                parsers.LogEntry
	primary, printed bool
	keys             []string
}
type Correlator struct {
	Limit  int
	Window time.Duration
	Fields []string
	Tokens bool
	items  []*item
	index  map[string]map[*item]struct{}
}

func (c *Correlator) keys(e parsers.LogEntry) []string {
	keys := map[string]bool{}
	if c.Tokens {
		for _, t := range e.Tokens {
			if t != "127.0.0.1" && t != "0.0.0.0" {
				keys["token:"+t] = true
			}
		}
	}
	// Comma-separated fields correlate independently; '=' aliases allow request_id=trace_id.
	for _, group := range c.Fields {
		for _, field := range strings.Split(group, "=") {
			if value := e.Field(field); value != "" {
				keys["field:"+group+":"+value] = true
			}
		}
	}
	out := make([]string, 0, len(keys))
	for k := range keys {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func (c *Correlator) nearby(a, b parsers.LogEntry) bool {
	if c.Window == 0 {
		return true
	}
	if a.Timestamp.IsZero() || b.Timestamp.IsZero() {
		return false
	}
	d := a.Timestamp.Sub(b.Timestamp)
	return d >= -c.Window && d <= c.Window
}

// Add returns each selected event at most once. All entries, including nonmatches,
// are indexed so that a later primary match can recover earlier context.
func (c *Correlator) Add(e parsers.LogEntry, primary bool) []parsers.LogEntry {
	if c.Limit <= 0 {
		c.Limit = 5000
	}
	if c.index == nil {
		c.index = map[string]map[*item]struct{}{}
	}
	n := &item{e: e, primary: primary, keys: c.keys(e)}
	linked := false
	prior := map[*item]bool{}
	for _, key := range n.keys {
		for old := range c.index[key] {
			if c.nearby(e, old.e) {
				if primary {
					prior[old] = true
				}
				if old.primary {
					linked = true
				}
			}
		}
	}
	var out []parsers.LogEntry
	for _, old := range c.items {
		if prior[old] && !old.printed {
			old.printed = true
			copy := old.e
			copy.Correlated = !old.primary
			out = append(out, copy)
		}
	}
	if primary || linked {
		n.printed = true
		copy := e
		copy.Correlated = !primary
		out = append(out, copy)
	}
	if len(c.items) >= c.Limit {
		old := c.items[0]
		for _, key := range old.keys {
			delete(c.index[key], old)
			if len(c.index[key]) == 0 {
				delete(c.index, key)
			}
		}
		copy(c.items, c.items[1:])
		c.items = c.items[:len(c.items)-1]
	}
	c.items = append(c.items, n)
	for _, key := range n.keys {
		if c.index[key] == nil {
			c.index[key] = map[*item]struct{}{}
		}
		c.index[key][n] = struct{}{}
	}
	return out
}
func Sort(entries []parsers.LogEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.Timestamp.Equal(b.Timestamp) {
			if a.Source == b.Source {
				return a.Line < b.Line
			}
			return a.Source < b.Source
		}
		if a.Timestamp.IsZero() {
			return false
		}
		if b.Timestamp.IsZero() {
			return true
		}
		return a.Timestamp.Before(b.Timestamp)
	})
}
func ParseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	t, e := time.Parse(time.RFC3339Nano, s)
	if e != nil {
		return t, fmt.Errorf("expected RFC3339 time: %w", e)
	}
	return t, nil
}
