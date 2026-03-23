package parsers

import (
	"encoding/json"
	"strings"
)

type GCPEngine struct{}

func NewGCPEngine() *GCPEngine {
	return &GCPEngine{}
}

func (g *GCPEngine) Parse(line string) (string, map[string]string) {
	if !strings.HasPrefix(strings.TrimSpace(line), "{") {
		return "", nil
	}
	
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(line), &data); err == nil {
		// GCP specific logging fields
		logName, hasLogName := data["logName"].(string)
		_, hasResource := data["resource"].(map[string]interface{})
		
		if hasLogName && hasResource {
			parsed := make(map[string]string)
			parsed["log_name"] = logName
			if severity, ok := data["severity"].(string); ok {
				parsed["severity"] = severity
			}
			
			// Extract structured payload if present
			if payload, ok := data["jsonPayload"].(map[string]interface{}); ok {
				if msg, ok := payload["message"].(string); ok {
					parsed["message"] = msg
				}
			} else if textPayload, ok := data["textPayload"].(string); ok {
				parsed["message"] = textPayload
			}
			return "gcp-cloud-logging", parsed
		}
	}
	return "", nil
}
