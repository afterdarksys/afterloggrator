package sources

import (
	"afterloggrator/parsers"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const maxResponse = 32 << 20

func denyRedirect(_ *http.Request, _ []*http.Request) error {
	return fmt.Errorf("redirect refused; configure the final HTTPS endpoint")
}
func httpLogs(ctx context.Context, s Source, opt Options, emit Emit) error {
	return queryHTTP(ctx, &http.Client{Timeout: 60 * time.Second, CheckRedirect: denyRedirect}, s, opt, emit)
}
func queryHTTP(ctx context.Context, client *http.Client, s Source, opt Options, emit Emit) error {
	endpoint := s.URL
	method := s.Method
	if method == "" {
		method = "GET"
	}
	body := map[string]any{}
	for k, v := range s.Body {
		body[k] = v
	}
	recordsPath, nextPath, pageParam := s.RecordsPath, s.NextTokenPath, s.PageParam
	switch s.Type {
	case "gcp":
		if endpoint == "" {
			endpoint = "https://logging.googleapis.com/v2/entries:list"
		}
		method = "POST"
		filter := s.Query
		for _, bound := range []struct {
			t  time.Time
			op string
		}{{opt.Since, ">="}, {opt.Until, "<="}} {
			if !bound.t.IsZero() {
				clause := fmt.Sprintf("timestamp %s %q", bound.op, bound.t.Format(time.RFC3339Nano))
				if filter != "" {
					filter = "(" + filter + ") AND " + clause
				} else {
					filter = clause
				}
			}
		}
		body = map[string]any{"resourceNames": []string{s.Resource}, "filter": filter, "orderBy": "timestamp asc", "pageSize": 1000}
		recordsPath = "entries"
		nextPath = "nextPageToken"
		pageParam = "pageToken"
	case "azure":
		if endpoint == "" {
			endpoint = "https://api.loganalytics.azure.com/v1/workspaces/" + url.PathEscape(s.Resource) + "/query"
		}
		method = "POST"
		body = map[string]any{"query": s.Query}
		if !opt.Since.IsZero() && !opt.Until.IsZero() {
			body["timespan"] = opt.Since.Format(time.RFC3339Nano) + "/" + opt.Until.Format(time.RFC3339Nano)
		}
	}
	seen := map[string]bool{}
	token := ""
	for {
		u, err := url.Parse(endpoint)
		if err != nil {
			return err
		}
		if token != "" {
			if method == "GET" {
				q := u.Query()
				q.Set(pageParam, token)
				u.RawQuery = q.Encode()
			} else {
				body[pageParam] = token
			}
		}
		var payload []byte
		if method == "POST" {
			payload, err = json.Marshal(body)
			if err != nil {
				return err
			}
		}
		req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/json")
		if method == "POST" {
			req.Header.Set("Content-Type", "application/json")
		}
		if s.TokenEnv != "" {
			req.Header.Set("Authorization", "Bearer "+os.Getenv(s.TokenEnv))
		}
		if s.APIKeyEnv != "" {
			h := s.APIKeyHeader
			if h == "" {
				h = "X-API-Key"
			}
			req.Header.Set(h, os.Getenv(s.APIKeyEnv))
		}
		for h, env := range s.HeadersEnv {
			req.Header.Set(h, os.Getenv(env))
		}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("HTTPS request failed: %w", redactURL(err))
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("HTTPS status %d", resp.StatusCode)
		}
		if readErr != nil {
			return readErr
		}
		if len(data) > maxResponse {
			return fmt.Errorf("response exceeds 32 MiB; narrow the query")
		}
		if s.Format == "text" || s.Format == "ndjson" {
			if nextPath != "" {
				return fmt.Errorf("pagination requires JSON format")
			}
			return scan(ctx, bytes.NewReader(data), s.Name, func(e parsers.LogEntry) error {
				if s.Format == "ndjson" && !json.Valid([]byte(e.Line)) {
					return fmt.Errorf("invalid NDJSON record")
				}
				return emit(e)
			})
		}
		var doc any
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber()
		if err = dec.Decode(&doc); err != nil {
			return fmt.Errorf("invalid JSON response: %w", err)
		}
		var trailing any
		if err = dec.Decode(&trailing); err != io.EOF {
			return fmt.Errorf("response must contain one JSON document")
		}
		if m, ok := doc.(map[string]any); ok {
			if e := m["error"]; e != nil && (s.Type == "gcp" || s.Type == "azure" || recordsPath != "") {
				return fmt.Errorf("server returned an error or partial result")
			}
		}
		if s.Type == "azure" {
			return azureRows(doc, emit)
		}
		records, exists := lookup(doc, recordsPath)
		if !exists {
			if s.Type == "gcp" {
				records = []any{}
			} else {
				return fmt.Errorf("records_path %q missing", recordsPath)
			}
		}
		switch rows := records.(type) {
		case []any:
			for _, row := range rows {
				if err = emitJSON(row, emit); err != nil {
					return err
				}
			}
		case map[string]any:
			if recordsPath != "" {
				return fmt.Errorf("records_path must identify an array")
			}
			if err = emitJSON(rows, emit); err != nil {
				return err
			}
		default:
			return fmt.Errorf("expected an array or JSON object")
		}
		if nextPath == "" {
			return nil
		}
		v, exists := lookup(doc, nextPath)
		if !exists || v == nil || v == "" {
			return nil
		}
		next, ok := v.(string)
		if !ok {
			return fmt.Errorf("pagination token must be a string")
		}
		if seen[next] {
			return fmt.Errorf("repeated pagination token")
		}
		seen[next] = true
		token = next
	}
}
func redactURL(err error) error {
	if e, ok := err.(*url.Error); ok {
		return e.Err
	}
	return err
}
func lookup(doc any, path string) (any, bool) {
	if path == "" {
		return doc, true
	}
	for _, key := range strings.Split(path, ".") {
		m, ok := doc.(map[string]any)
		if !ok {
			return nil, false
		}
		doc, ok = m[key]
		if !ok {
			return nil, false
		}
	}
	return doc, true
}
func emitJSON(row any, emit Emit) error {
	switch v := row.(type) {
	case string:
		return emit(parsers.LogEntry{Line: v})
	case map[string]any:
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		return emit(parsers.LogEntry{Line: string(b)})
	default:
		return fmt.Errorf("log record must be a string or object")
	}
}
func azureRows(doc any, emit Emit) error {
	v, ok := lookup(doc, "tables")
	if !ok {
		return fmt.Errorf("Azure response missing tables")
	}
	tables, ok := v.([]any)
	if !ok {
		return fmt.Errorf("invalid Azure tables")
	}
	for _, t := range tables {
		m, ok := t.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid Azure table")
		}
		columns, ok := m["columns"].([]any)
		if !ok {
			return fmt.Errorf("invalid Azure columns")
		}
		rows, ok := m["rows"].([]any)
		if !ok {
			return fmt.Errorf("invalid Azure rows")
		}
		names := make([]string, len(columns))
		for i, c := range columns {
			col, ok := c.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid Azure column")
			}
			names[i], ok = col["name"].(string)
			if !ok || names[i] == "" {
				return fmt.Errorf("invalid Azure column name")
			}
		}
		for _, r := range rows {
			cells, ok := r.([]any)
			if !ok || len(cells) != len(names) {
				return fmt.Errorf("Azure row/column count mismatch")
			}
			obj := map[string]any{}
			for i, c := range cells {
				obj[names[i]] = c
			}
			if err := emitJSON(obj, emit); err != nil {
				return err
			}
		}
	}
	return nil
}
