package parsers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var keyValue = regexp.MustCompile(`(?:^|\s)([\w.-]+)=("(?:[^"\\]|\\.)*"|'[^']*'|\S+)`)

// Normalize preserves the original line and flattens structured fields using dotted paths.
// Timestamp is event time; unknown event times stay zero, never masquerading as ingestion time.
func Normalize(e *LogEntry, engines []Engine) {
	if e.ObservedAt.IsZero() {
		e.ObservedAt = time.Now().UTC()
	}
	if e.ParsedFields == nil {
		e.ParsedFields = map[string]string{}
	}
	var obj map[string]any
	decoder := json.NewDecoder(bytes.NewBufferString(e.Line))
	decoder.UseNumber()
	if decoder.Decode(&obj) == nil {
		flatten("", obj, e.ParsedFields)
	}
	for _, m := range keyValue.FindAllStringSubmatch(e.Line, -1) {
		v := m[2]
		if unquoted, err := strconv.Unquote(v); err == nil {
			v = unquoted
		} else {
			v = strings.Trim(v, "'\"")
		}
		e.ParsedFields[m[1]] = v
	}
	for _, engine := range engines {
		app, fields := engine.Parse(e.Line)
		if app != "" {
			if e.AppID == "" {
				e.AppID = app
			}
			for k, v := range fields {
				e.ParsedFields[k] = v
			}
			break
		}
	}
	pick := func(keys ...string) string {
		for _, k := range keys {
			if v := e.ParsedFields[k]; v != "" {
				return v
			}
		}
		return ""
	}
	if e.Host == "" {
		e.Host = pick("host", "hostname", "Computer", "resource.labels.node_name", "kubernetes.host")
	}
	if e.Service == "" {
		e.Service = pick("service.name", "service", "app", "resource.labels.service_name", "resource.labels.container_name")
	}
	if e.Container == "" {
		e.Container = pick("container.id", "container_id", "container", "resource.labels.container_name", "kubernetes.container_name")
	}
	if e.Cluster == "" {
		e.Cluster = pick("cluster", "cluster.name", "resource.labels.cluster_name", "kubernetes.cluster_name")
	}
	if e.Timestamp.IsZero() {
		for _, k := range []string{"timestamp", "@timestamp", "time", "eventTime", "TimeGenerated", "datetime", "data.datetime", "logContent.time", "data.logContent.time"} {
			if t, ok := EventTime(e.ParsedFields[k]); ok {
				e.Timestamp = t
				break
			}
		}
	}
	parts := strings.Fields(e.Line)
	// RFC5424: PRI/version, timestamp, hostname, app-name, procid, msgid.
	if len(parts) >= 6 && strings.HasPrefix(parts[0], "<") && strings.HasSuffix(parts[0], ">1") {
		if e.Timestamp.IsZero() {
			if ts, ok := EventTime(parts[1]); ok {
				e.Timestamp = ts
			}
		}
		if e.Host == "" && parts[2] != "-" {
			e.Host = parts[2]
		}
		if e.Service == "" && parts[3] != "-" {
			e.Service = parts[3]
		}
	}
	if e.Timestamp.IsZero() {
		parts := strings.Fields(e.Line)
		if len(parts) > 0 {
			if t, ok := EventTime(parts[0]); ok {
				e.Timestamp = t
			}
		}
	}
	e.Tokens = FindTokensFast(e.Line)
}
func flatten(prefix string, value any, dst map[string]string) {
	switch v := value.(type) {
	case map[string]any:
		for k, child := range v {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			flatten(key, child, dst)
		}
	case []any:
		b, _ := json.Marshal(v)
		dst[prefix] = string(b)
	case nil:
	default:
		dst[prefix] = fmt.Sprint(v)
	}
}
func EventTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC(), true
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		switch {
		case n >= 1e17:
			return time.Unix(0, n).UTC(), true
		case n >= 1e14:
			return time.UnixMicro(n).UTC(), true
		case n >= 1e11:
			return time.UnixMilli(n).UTC(), true
		case n >= 1e9:
			return time.Unix(n, 0).UTC(), true
		}
	}
	return time.Time{}, false
}
func (e LogEntry) Field(key string) string {
	switch key {
	case "source":
		return e.Source
	case "host":
		return e.Host
	case "service":
		return e.Service
	case "container":
		return e.Container
	case "cluster":
		return e.Cluster
	case "app", "app_id":
		return e.AppID
	case "line":
		return e.Line
	case "filename":
		return e.Filename
	}
	return e.ParsedFields[key]
}
