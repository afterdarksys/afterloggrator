package sources

import (
	"afterloggrator/parsers"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGCPPaginationAndAuth(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("missing authentication")
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if !strings.Contains(fmt.Sprint(body["filter"]), "timestamp >=") {
			t.Error("missing time filter")
		}
		if calls == 1 {
			fmt.Fprint(w, `{"entries":[],"nextPageToken":"next"}`)
		} else {
			if body["pageToken"] != "next" {
				t.Error("missing page token")
			}
			fmt.Fprint(w, `{"entries":[{"timestamp":"2026-01-01T00:00:01Z","textPayload":"test"}]}`)
		}
	}))
	defer server.Close()
	t.Setenv("TEST_TOKEN", "secret")
	var got []parsers.LogEntry
	err := queryHTTP(context.Background(), server.Client(), Source{Type: "gcp", URL: server.URL, Resource: "projects/test", TokenEnv: "TEST_TOKEN"}, Options{Since: time.Now()}, func(e parsers.LogEntry) error { got = append(got, e); return nil })
	if err != nil || calls != 2 || len(got) != 1 {
		t.Fatalf("%v %d %v", err, calls, got)
	}
}
func TestHTTPSKeyAndPagination(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-API-Key") != "key" {
			t.Error("missing key")
		}
		if calls == 1 {
			fmt.Fprint(w, `{"data":{"logs":["one"]},"next":"p2"}`)
		} else {
			if r.URL.Query().Get("cursor") != "p2" {
				t.Error("wrong cursor")
			}
			fmt.Fprint(w, `{"data":{"logs":["two"]}}`)
		}
	}))
	defer server.Close()
	t.Setenv("TEST_KEY", "key")
	var lines []string
	err := queryHTTP(context.Background(), server.Client(), Source{Type: "darkapi", URL: server.URL, APIKeyEnv: "TEST_KEY", RecordsPath: "data.logs", NextTokenPath: "next", PageParam: "cursor"}, Options{}, func(e parsers.LogEntry) error { lines = append(lines, e.Line); return nil })
	if err != nil || strings.Join(lines, ",") != "one,two" {
		t.Fatalf("%v %v", err, lines)
	}
}
func TestAzureTablesAndPartialErrors(t *testing.T) {
	for _, tc := range []struct {
		body string
		fail bool
	}{{`{"tables":[{"columns":[{"name":"TimeGenerated"},{"name":"Message"}],"rows":[["2026-01-01T00:00:00Z","hello"]]}]}`, false}, {`{"tables":[],"error":{"code":"PartialError"}}`, true}, {`{"tables":[{"columns":[{"name":"x"}],"rows":[[]]}]}`, true}, {`{}`, true}} {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, tc.body) }))
		count := 0
		err := queryHTTP(context.Background(), server.Client(), Source{Type: "azure", URL: server.URL, Resource: "workspace", Query: "AppTraces"}, Options{}, func(e parsers.LogEntry) error { count++; return nil })
		server.Close()
		if (err != nil) != tc.fail {
			t.Fatalf("%s: %v", tc.body, err)
		}
		if !tc.fail && count != 1 {
			t.Fatal(count)
		}
	}
}
func TestHTTPFailures(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{{401, `secret must not leak`}, {200, `{"logs":[],"next":"same"}`}, {200, `{"unexpected":[]}`}, {200, `<html>login</html>`}} {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
		err := queryHTTP(context.Background(), server.Client(), Source{Type: "https", URL: server.URL, RecordsPath: "logs", NextTokenPath: "next", PageParam: "page"}, Options{}, func(parsers.LogEntry) error { return nil })
		server.Close()
		if err == nil || strings.Contains(err.Error(), "secret must not leak") {
			t.Fatalf("%v", err)
		}
	}
}
func TestTLSAndRedirectRefused(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "https://example.com", 302) }))
	defer server.Close()
	if err := httpLogs(context.Background(), Source{Type: "https", URL: server.URL}, Options{}, func(parsers.LogEntry) error { return nil }); err == nil {
		t.Fatal("untrusted certificate accepted")
	}
	client := server.Client()
	client.CheckRedirect = denyRedirect
	if err := queryHTTP(context.Background(), client, Source{Type: "https", URL: server.URL}, Options{}, func(parsers.LogEntry) error { return nil }); err == nil {
		t.Fatal("redirect accepted")
	}
}
func TestConfigValidation(t *testing.T) {
	for _, s := range []Source{{Name: "x", Type: "https", URL: "http://example.com"}, {Name: "x", Type: "https", URL: "https://user:pass@example.com"}, {Name: "x", Type: "ssh", Host: "h", Path: "p"}, {Name: "x", Type: "gcp", Resource: "projects/p", TokenEnv: "MISSING_TEST_TOKEN"}, {Name: "x", Type: "https", URL: "https://example.com", NextTokenPath: "next"}} {
		if Validate([]Source{s}) == nil {
			t.Fatalf("accepted %+v", s)
		}
	}
}
