package main

import (
	"afterloggrator/parsers"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMultiSourceSearch(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.log")
	b := filepath.Join(dir, "b.log")
	os.WriteFile(a, []byte("{\"timestamp\":\"2026-01-01T00:00:01Z\",\"service\":\"api\",\"request_id\":\"r1\",\"message\":\"start\"}\n"), 0600)
	os.WriteFile(b, []byte("{\"timestamp\":\"2026-01-01T00:00:02Z\",\"service\":\"db\",\"trace_id\":\"r1\",\"message\":\"failure\"}\n"), 0600)
	cfg := filepath.Join(dir, "sources.json")
	data, _ := json.Marshal(map[string]any{"sources": []map[string]string{{"name": "a", "type": "file", "path": a, "host": "h1", "cluster": "c1", "container": "pod1"}, {"name": "b", "type": "file", "path": b, "host": "h2", "cluster": "c2", "container": "pod2"}}})
	os.WriteFile(cfg, data, 0600)
	var out, stderr bytes.Buffer
	err := run(context.Background(), []string{"-sources", cfg, "-contains", "failure", "-correlate-by", "request_id=trace_id", "-sort", "-json", "-since", "2026-01-01T00:00:00Z"}, &out, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(&out)
	var entries []parsers.LogEntry
	for dec.More() {
		var e parsers.LogEntry
		if err = dec.Decode(&e); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, e)
	}
	if len(entries) != 2 || entries[0].Host != "h1" || entries[1].Cluster != "c2" || !entries[0].Correlated {
		t.Fatalf("unexpected results: %+v", entries)
	}
}
func TestMissingSourceFails(t *testing.T) {
	var out, errout bytes.Buffer
	err := run(context.Background(), []string{"-json", "/missing/log"}, &out, &errout)
	if err == nil || !strings.Contains(err.Error(), "source") {
		t.Fatal(err)
	}
}
func TestSortLimitFailsWithoutPartialOutput(t *testing.T) {
	p := filepath.Join(t.TempDir(), "log")
	os.WriteFile(p, []byte("a\nb\n"), 0600)
	var out bytes.Buffer
	err := run(context.Background(), []string{"-sort", "-max-results", "1", p}, &out, &out)
	if err == nil || out.Len() != 0 {
		t.Fatalf("%v %q", err, out.String())
	}
}
func TestFollowConcurrencyValidation(t *testing.T) {
	var out bytes.Buffer
	if err := run(context.Background(), []string{"-f", "-concurrency", "1", "a", "b"}, &out, &out); err == nil {
		t.Fatal("would starve second source")
	}
}
