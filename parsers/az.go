package parsers

import (
	"encoding/json"
	"strings"
)

type AzureEngine struct{}

func NewAzureEngine() *AzureEngine {
	return &AzureEngine{}
}

func (a *AzureEngine) Parse(line string) (string, map[string]string) {
	// Azure standard JSON format for Resource Logs / Activity Logs
	if strings.HasPrefix(strings.TrimSpace(line), "{") {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(line), &data); err == nil {
			// Typical Azure AD or Monitor indicator
			parsed := make(map[string]string)

			if tenant, ok := data["tenantId"].(string); ok {
				parsed["tenant_id"] = tenant
				if opName, ok := data["operationName"].(string); ok {
					parsed["operation"] = opName
				}
				if resId, ok := data["resourceId"].(string); ok {
					parsed["resource"] = resId
				}
				return "azure-monitor", parsed
			}

			// Azure App Service
			if level, ok := data["Level"].(string); ok {
				if msg, ok := data["Message"].(string); ok && strings.Contains(line, "w3wp") {
					parsed["level"] = level
					parsed["msg"] = msg
					return "azure-appservice", parsed
				}
			}
		}
	}
	return "", nil
}
