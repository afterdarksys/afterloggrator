package parsers

import (
	"encoding/json"
	"strings"
)

type OCIEngine struct{}

func NewOCIEngine() *OCIEngine {
	return &OCIEngine{}
}

func (o *OCIEngine) Parse(line string) (string, map[string]string) {
	if !strings.HasPrefix(strings.TrimSpace(line), "{") {
		return "", nil
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(line), &data); err == nil {
		// OCI typically encapsulates wrapped JSON data inside standard Oracle keys
		source, hasSource := data["source"].(string)
		typeField, hasType := data["type"].(string)
		
		if hasSource && hasType && strings.Contains(typeField, "com.oraclecloud") {
			parsed := make(map[string]string)
			parsed["source"] = source
			parsed["oracle_type"] = typeField
			
			if oracleTime, ok := data["time"].(string); ok {
				parsed["oci_time"] = oracleTime
			}
			
			if dataObj, ok := data["data"].(map[string]interface{}); ok {
				if message, ok := dataObj["message"].(string); ok {
					parsed["oci_msg"] = message
				}
			}
			
			return "oci-cloud", parsed
		}
	}
	return "", nil
}
