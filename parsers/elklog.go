package parsers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type ElkEngine struct {
	URL    string
	Index  string
	User   string
	Pass   string
	Client *http.Client
}

func NewElkEngine(url, index, user, pass string) *ElkEngine {
	return &ElkEngine{
		URL:   url,
		Index: index,
		User:  user,
		Pass:  pass,
		Client: &http.Client{
			Timeout:       10 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return fmt.Errorf("redirect refused") },
		},
	}
}

func (e *ElkEngine) Query(token string, logChan chan<- LogEntry) {

	if e.URL == "" || e.Index == "" {
		return
	}

	queryBody := map[string]interface{}{
		"query": map[string]interface{}{
			"match_phrase": map[string]interface{}{
				"message": token,
			},
		},
		"size": 100,
	}

	b, _ := json.Marshal(queryBody)
	url := fmt.Sprintf("%s/%s/_search", strings.TrimRight(e.URL, "/"), e.Index)
	req, err := http.NewRequest("POST", url, bytes.NewReader(b))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if e.User != "" && e.Pass != "" {
		req.SetBasicAuth(e.User, e.Pass)
	}

	resp, err := e.Client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return
	}

	var esResp struct {
		Hits struct {
			Hits []struct {
				Source map[string]interface{} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&esResp); err != nil {
		return
	}

	for _, hit := range esResp.Hits.Hits {
		var line string
		if msg, ok := hit.Source["message"]; ok && msg != nil {
			line = fmt.Sprintf("%v", msg)
		} else {
			bd, _ := json.Marshal(hit.Source)
			line = string(bd)
		}

		timestamp := time.Now()
		if ts, ok := hit.Source["@timestamp"].(string); ok {
			if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
				timestamp = parsed
			}
		}

		logChan <- LogEntry{
			Filename:  "Elasticsearch",
			Line:      strings.TrimSpace(line),
			Timestamp: timestamp,
			Color:     ColorBlue,
			AppID:     "elasticsearch",
		}
	}
}
