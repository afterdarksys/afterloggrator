package plugins

import (
	"afterloggrator/parsers"
	"regexp"
	"strings"
)

type PaloAltoPlugin struct {
	trafficRe *regexp.Regexp
}

func (p *PaloAltoPlugin) Init() error {
	// PAN-OS CSV Syslog roughly standardizes to:
	// Domain,ReceiveTime,Serial,Type,Threat/ContentType,ConfigVersion,GenerateTime,SourceAddress,DestinationAddress,NATSourceIP,NATDestinationIP,RuleName...
	p.trafficRe = regexp.MustCompile(`(?i)(TRAFFIC|THREAT|SYSTEM),.*?,\d+,.*?,.*?,.*?,(?P<src_ip>[0-9\.]+),(?P<dst_ip>[0-9\.]+),.*?,\w+,(?P<sport>\d+),(?P<dport>\d+)`)
	return nil
}

func (p *PaloAltoPlugin) Process(entry *parsers.LogEntry) {
	if strings.Contains(entry.Line, "TRAFFIC") || strings.Contains(entry.Line, "THREAT") {
		// Example extraction to structurally tag Palo Alto syslog streams seamlessly
		if entry.ParsedFields == nil {
			entry.ParsedFields = make(map[string]string)
		}
		
		names := p.trafficRe.SubexpNames()
		matches := p.trafficRe.FindStringSubmatch(entry.Line)
		
		if len(matches) > 0 {
			entry.AppID = "palo-alto-panos"
			for i, match := range matches {
				name := names[i]
				if name != "" && match != "" {
					entry.ParsedFields[name] = match
				}
			}
		}
	}
}
