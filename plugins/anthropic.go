package plugins

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"afterloggrator/parsers"
	"net/http"
	"os"
	"strings"
	"time"
)

type ClaudeDiagnostic struct {
	Enabled bool
}

func (c *ClaudeDiagnostic) RunDiagnostics(logs []string) {
	if !c.Enabled || len(logs) == 0 {
		return
	}

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		safePrint("\n%sError: ANTHROPIC_API_KEY environment variable is required for --claude feature%s\n", parsers.ColorRed, parsers.ColorReset)
		return
	}

	safePrint("\n%s🤖 Sending aggregated logs to Claude for analysis...%s\n", parsers.ColorPurple, parsers.ColorReset)

	prompt := "Please analyze the following correlated logs. Identify any core root causes, potential issues, and recommend troubleshooting steps. Be concise and format your response in markdown.\n\n"
	prompt += strings.Join(logs, "\n")

	reqBody := map[string]interface{}{
		"model":      "claude-3-7-sonnet-20250219",
		"max_tokens": 1500,
		"stream":     true,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}

	b, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(b))
	if err != nil {
		safePrint("Error creating request: %v\n", err)
		return
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		safePrint("Error calling Anthropic API: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		safePrint("Anthropic API Error (%d): %s\n", resp.StatusCode, string(body))
		return
	}

	safePrint("%s=== Claude AI Diagnosis ===%s\n", parsers.ColorPurple, parsers.ColorReset)

	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				safePrint("Error reading stream: %v\n", err)
			}
			break
		}

		if len(line) == 0 || line[0] == '\n' {
			continue
		}

		if bytes.HasPrefix(line, []byte("data: ")) {
			data := bytes.TrimPrefix(line, []byte("data: "))
			if bytes.HasPrefix(data, []byte("[DONE]")) {
				break
			}

			var evt struct {
				Type  string `json:"type"`
				Delta struct {
					Text string `json:"text"`
				} `json:"delta"`
			}
			if err := json.Unmarshal(data, &evt); err == nil {
				if evt.Type == "content_block_delta" {
					safePrint("%s", evt.Delta.Text)
				}
			}
		}
	}
	safePrint("\n%s===========================%s\n", parsers.ColorPurple, parsers.ColorReset)
}

func safePrint(format string, args ...interface{}) {
	fmt.Printf(format, args...)
}
