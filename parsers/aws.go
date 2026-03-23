package parsers

import (
	"encoding/json"
	"strings"
)

type AWSEngine struct{}

func NewAWSEngine() *AWSEngine {
	return &AWSEngine{}
}

func (a *AWSEngine) Parse(line string) (string, map[string]string) {
	// Attempt AWS CloudTrail or CloudWatch JSON parse
	if strings.HasPrefix(strings.TrimSpace(line), "{") {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(line), &data); err == nil {
			if eventSource, ok := data["eventSource"].(string); ok && strings.HasSuffix(eventSource, "amazonaws.com") {
				parsed := make(map[string]string)
				if eventName, ok := data["eventName"].(string); ok {
					parsed["event_name"] = eventName
				}
				if userIdentity, ok := data["userIdentity"].(map[string]interface{}); ok {
					if arn, ok := userIdentity["arn"].(string); ok {
						parsed["user_arn"] = arn
					}
				}
				return "aws-cloudtrail", parsed
			}
		}
	}

	// Attempt AWS VPC Flow Log
	// account-id interface-id srcaddr dstaddr srcport dstport protocol packets bytes start end action log-status
	if len(strings.Split(line, " ")) >= 14 && strings.Contains(line, " ACCEPT ") || strings.Contains(line, " REJECT ") {
		parts := strings.Split(strings.TrimSpace(line), " ")
		if len(parts) >= 14 {
			parsed := map[string]string{
				"account_id": parts[1],
				"src_ip":     parts[3],
				"dst_ip":     parts[4],
				"src_port":   parts[5],
				"dst_port":   parts[6],
				"action":     parts[12],
			}
			return "aws-vpc-flow", parsed
		}
	}
	return "", nil
}
