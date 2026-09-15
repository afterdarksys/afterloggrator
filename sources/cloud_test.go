package sources

import (
	"afterloggrator/parsers"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/loggingsearch"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAWSSignedPaginatedSearch(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
			t.Error("unsigned AWS request")
		}
		if r.Header.Get("X-Amz-Target") != "Logs_20140328.FilterLogEvents" {
			t.Error("wrong operation")
		}
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		if req["logGroupName"] != "group" || req["filterPattern"] != "ERROR" || req["startTime"] == nil {
			t.Errorf("wrong request: %v", req)
		}
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		if calls == 1 {
			fmt.Fprint(w, `{"events":[],"nextToken":"second"}`)
		} else {
			if req["nextToken"] != "second" {
				t.Error("missing cursor")
			}
			fmt.Fprint(w, `{"events":[{"eventId":"id","message":"ERROR","logStreamName":"pod-a","timestamp":1767225600000}]}`)
		}
	}))
	defer server.Close()
	client := cloudwatchlogs.NewFromConfig(aws.Config{Region: "us-east-1", Credentials: credentials.NewStaticCredentialsProvider("key", "secret", "session"), HTTPClient: server.Client()}, func(o *cloudwatchlogs.Options) { o.BaseEndpoint = aws.String(server.URL) })
	var got []parsers.LogEntry
	err := queryAWS(context.Background(), client, Source{Resource: "group", Query: "ERROR"}, Options{Since: time.Now()}, func(e parsers.LogEntry) error { got = append(got, e); return nil })
	if err != nil || calls != 2 || len(got) != 1 || got[0].Timestamp.IsZero() || got[0].ParsedFields["log_stream"] != "pod-a" {
		t.Fatalf("%v %+v calls=%d", err, got, calls)
	}
}
func TestOCISignedPaginatedSearch(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if !strings.Contains(r.Header.Get("Authorization"), `algorithm="rsa-sha256"`) {
			t.Error("unsigned OCI request")
		}
		if r.Header.Get("X-Content-Sha256") == "" {
			t.Error("missing signed body digest")
		}
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		if req["searchQuery"] != `search "ocid1.compartment.test"` || req["timeStart"] == nil || req["timeEnd"] == nil {
			t.Errorf("invalid search body: %v", req)
		}
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			w.Header().Set("opc-next-page", "next")
			fmt.Fprint(w, `{"summary":{},"results":[]}`)
		} else {
			if r.URL.Query().Get("page") != "next" {
				t.Error("missing page")
			}
			fmt.Fprint(w, `{"summary":{},"results":[{"data":{"datetime":1767225600000,"logContent":{"data":{"message":"hello"}}}}]}`)
		}
	}))
	defer server.Close()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	provider := common.NewRawConfigurationProvider("ocid1.tenancy.test", "ocid1.user.test", "us-ashburn-1", "fingerprint", string(keyPEM), nil)
	client, err := loggingsearch.NewLogSearchClientWithConfigurationProvider(provider)
	if err != nil {
		t.Fatal(err)
	}
	client.Host = server.URL
	client.HTTPClient = server.Client()
	count := 0
	err = queryOCI(context.Background(), client, Source{Query: `search "ocid1.compartment.test"`}, Options{Since: time.Now().Add(-time.Hour), Until: time.Now()}, func(e parsers.LogEntry) error {
		count++
		parsers.Normalize(&e, nil)
		if e.Timestamp.IsZero() {
			t.Error("OCI timestamp missing")
		}
		return nil
	})
	if err != nil || count != 1 || calls != 2 {
		t.Fatalf("%v count=%d calls=%d", err, count, calls)
	}
}
