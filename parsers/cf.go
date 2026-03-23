package parsers

import (
	"encoding/json"
	"strings"
)

type CloudflareEngine struct{}

func NewCloudflareEngine() *CloudflareEngine {
	return &CloudflareEngine{}
}

func (c *CloudflareEngine) Parse(line string) (string, map[string]string) {
	if !strings.HasPrefix(strings.TrimSpace(line), "{") {
		return "", nil
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(line), &data); err == nil {
		// Cloudflare specific logpush format (often Capitalized keys)
		rayID, hasRayID := data["RayID"].(string)
		clientIP, hasIP := data["ClientIP"].(string)
		
		if hasRayID && hasIP {
			parsed := make(map[string]string)
			parsed["ray_id"] = rayID
			parsed["client_ip"] = clientIP
			
			if edgeResp, ok := data["EdgeResponseStatus"].(float64); ok {
				// We can format integers out
				if edgeResp > 0 {
					parsed["status"] = "edge-served"
				}
			}
			if host, ok := data["ClientRequestHost"].(string); ok {
				parsed["host"] = host
			}
			if path, ok := data["ClientRequestURI"].(string); ok {
				parsed["uri"] = path
			}
			return "cloudflare-worker", parsed
		}
	}
	return "", nil
}
