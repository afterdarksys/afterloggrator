package parsers

import (
	"testing"
	"time"
)

func TestNormalizeIdentityTimestampPrecision(t *testing.T) {
	e := LogEntry{Source: "gcp", Line: `{"timestamp":"2026-01-01T02:00:00+02:00","jsonPayload":{"request_id":9007199254740993},"resource":{"labels":{"cluster_name":"prod","container_name":"web","node_name":"node-a"}}}`}
	Normalize(&e, nil)
	if e.Cluster != "prod" || e.Host != "node-a" || e.Container != "web" {
		t.Fatalf("identity: %+v", e)
	}
	if e.ParsedFields["jsonPayload.request_id"] != "9007199254740993" {
		t.Fatal("lost numeric precision")
	}
	if !e.Timestamp.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatal(e.Timestamp)
	}
}
func TestUnknownTimeAndLogfmt(t *testing.T) {
	e := LogEntry{Line: `service=api request_id="two words" status=500`}
	Normalize(&e, nil)
	if !e.Timestamp.IsZero() || e.ObservedAt.IsZero() {
		t.Fatal("invented event time")
	}
	if e.Service != "api" || e.ParsedFields["request_id"] != "two words" {
		t.Fatal(e)
	}
}
func TestTokens(t *testing.T) {
	got := FindTokensFast(`ip=10.0.0.1:443 ipv6=[2001:db8::1] mac=AA:BB:CC:DD:EE:FF domain=EXAMPLE.COM bad=999.0.0.1`)
	if len(got) != 4 {
		t.Fatal(got)
	}
	if IsIPv4Fast("999999999999999999999999.0.0.1") {
		t.Fatal("overflow address accepted")
	}
}
func TestAWSFalsePositive(t *testing.T) {
	if app, _ := NewAWSEngine().Parse("a b c d e f g h i j k l REJECT m"); app != "" {
		t.Fatal("false VPC classification")
	}
}
