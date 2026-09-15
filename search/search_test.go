package search

import (
	"afterloggrator/parsers"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func event(source, id string, second int) parsers.LogEntry {
	return parsers.LogEntry{Source: source, Line: source, Timestamp: time.Date(2026, 1, 1, 0, 0, second, 0, time.UTC), ParsedFields: map[string]string{"request_id": id}}
}
func TestCorrelationBothDirectionsAndEviction(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		c := Correlator{Limit: 2, Window: time.Minute, Fields: []string{"request_id=trace_id"}}
		a := event("host-a", "42", 1)
		b := event("host-b", "", 2)
		b.ParsedFields = map[string]string{"trace_id": "42"}
		var got []parsers.LogEntry
		if reverse {
			got = append(got, c.Add(b, true)...)
			got = append(got, c.Add(a, false)...)
		} else {
			got = append(got, c.Add(a, false)...)
			got = append(got, c.Add(b, true)...)
		}
		if len(got) != 2 {
			t.Fatalf("reverse=%v: got %v", reverse, got)
		}
		if got[0].Source == got[1].Source {
			t.Fatal("duplicate event")
		}
		c.Add(event("c", "99", 3), false)
		if len(c.items) != 2 {
			t.Fatal("unbounded history")
		}
		for _, items := range c.index {
			for old := range items {
				if old.e.Source == got[0].Source {
					t.Fatal("stale index entry")
				}
			}
		}
	}
}
func TestCorrelationTimeAndExactValues(t *testing.T) {
	c := Correlator{Limit: 10, Window: time.Second, Fields: []string{"request_id"}}
	c.Add(event("a", "42", 1), true)
	if got := c.Add(event("b", "42", 5), false); len(got) != 0 {
		t.Fatal("outside window linked")
	}
	if got := c.Add(event("c", "142", 1), false); len(got) != 0 {
		t.Fatal("substring linked")
	}
	unknown := event("d", "42", 1)
	unknown.Timestamp = time.Time{}
	if len(c.Add(unknown, false)) != 0 {
		t.Fatal("unknown event time linked")
	}
}
func TestQueryRange(t *testing.T) {
	q := Query{Since: event("", "", 1).Timestamp, Until: event("", "", 3).Timestamp, Contains: []string{"error", "checkout"}, Fields: map[string]string{"host": "h"}}
	e := event("", "", 2)
	e.Line = "checkout error"
	e.Host = "h"
	ok, err := q.Match(e)
	if !ok || err != nil || !q.InRange(e) {
		t.Fatal("expected match")
	}
	e.Timestamp = time.Time{}
	if q.InRange(e) {
		t.Fatal("missing timestamp passed")
	}
}
func TestStarlarkSharedModulesAndFailures(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "helpers.star"), []byte("def wanted(value):\n    return value == 'payments'\n"), 0600)
	path := filepath.Join(dir, "filter.star")
	os.WriteFile(path, []byte("load('helpers.star', 'wanted')\ndef filter(event):\n    return wanted(event['service']) and event['fields'].get('status') == '500'\n"), 0600)
	s, err := LoadScript(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := s.Match(parsers.LogEntry{Service: "payments", ParsedFields: map[string]string{"status": "500"}})
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	for _, src := range []string{"def filter(e):\n    return 1\n", "def filter(e):\n    return read_file('/etc/passwd')\n", "def filter(e):\n    x = 0\n    for i in range(1000000):\n        x += i\n    return True\n"} {
		os.WriteFile(path, []byte(src), 0600)
		s, err = LoadScript(context.Background(), path)
		if err == nil {
			_, err = s.Match(parsers.LogEntry{})
		}
		if err == nil {
			t.Fatalf("expected failure: %s", src)
		}
	}
	os.WriteFile(path, []byte("load('../escape.star', 'x')\ndef filter(e):\n    return True\n"), 0600)
	if _, err = LoadScript(context.Background(), path); err == nil {
		t.Fatal("load escaped")
	}
}
func TestStarlarkSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.star")
	os.WriteFile(outside, []byte("x = True\n"), 0600)
	os.Symlink(outside, filepath.Join(dir, "escape.star"))
	path := filepath.Join(dir, "main.star")
	os.WriteFile(path, []byte("load('escape.star', 'x')\ndef filter(e):\n    return x\n"), 0600)
	_, err := LoadScript(context.Background(), path)
	if err == nil {
		t.Fatal("symlink escaped")
	}
}
func TestStarlarkCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	path := filepath.Join(t.TempDir(), "f.star")
	os.WriteFile(path, []byte("def filter(e):\n    return True\n"), 0600)
	s, err := LoadScript(ctx, path)
	if err == nil {
		_, err = s.Match(parsers.LogEntry{})
	}
	if err == nil || !strings.Contains(err.Error(), "cancel") {
		t.Fatalf("expected cancellation: %v", err)
	}
}
