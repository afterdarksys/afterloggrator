package plugins

import (
	"afterloggrator/parsers"
	"regexp"
	"strings"
)

type ProofpointPlugin struct {
	kvRe *regexp.Regexp
}

func (p *ProofpointPlugin) Init() error {
	// Proofpoint usually operates in Key-Value syslog streams:
	// filter_score=1.00 pps_cid=12312 module=mta rule=quarantine
	p.kvRe = regexp.MustCompile(`(?i)\b(filter_score|pps_cid|module|rule|action)=([^\s]+)`)
	return nil
}

func (p *ProofpointPlugin) Process(entry *parsers.LogEntry) {
	if strings.Contains(entry.Line, "pps_cid=") || strings.Contains(entry.Line, "filter_score=") {
		if entry.ParsedFields == nil {
			entry.ParsedFields = make(map[string]string)
		}
		entry.AppID = "proofpoint-pps"
		
		matches := p.kvRe.FindAllStringSubmatch(entry.Line, -1)
		for _, m := range matches {
			if len(m) == 3 {
				entry.ParsedFields[m[1]] = m[2]
			}
		}
	}
}
