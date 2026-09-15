package parsers

import (
	"encoding/json"
	"net"
	"strconv"
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

	// Default VPC Flow Log format: validate identifiers and addresses to avoid
	// classifying arbitrary text containing ACCEPT or REJECT as an AWS event.
	parts := strings.Fields(line)
	if len(parts) == 14 {
		version, e := strconv.Atoi(parts[0])
		_, accountErr := strconv.ParseUint(parts[1], 10, 64)
		if e == nil && version >= 2 && version <= 5 && len(parts[1]) == 12 && accountErr == nil && strings.HasPrefix(parts[2], "eni-") && net.ParseIP(parts[3]) != nil && net.ParseIP(parts[4]) != nil && (parts[12] == "ACCEPT" || parts[12] == "REJECT") {
			return "aws-vpc-flow", map[string]string{"account_id": parts[1], "src_ip": parts[3], "dst_ip": parts[4], "src_port": parts[5], "dst_port": parts[6], "action": parts[12]}
		}
	}
	return "", nil
}
