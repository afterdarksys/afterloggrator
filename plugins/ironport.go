package plugins

import (
	"afterloggrator/parsers"
	"regexp"
	"strings"
)

type IronportPlugin struct {
	wsaRe *regexp.Regexp
	esaRe *regexp.Regexp
}

func (p *IronportPlugin) Init() error {
	// Web Security Appliance (WSA) Squid-like format:
	// timestamp duration client_ip HTTP_CODE/STATUS size METHOD URL - HIERARCHY/server_ip contentType
	p.wsaRe = regexp.MustCompile(`(?P<duration>\d+) (?P<client_ip>[0-9\.]+) (?P<action>[A-Z_]+)/(?P<http_code>\d+) (?P<size>\d+) (?P<method>[A-Z]+) (?P<url>\S+) - (?P<hierarchy>\S+)/(?P<server_ip>[0-9\.]+|-|.*) (?P<content_type>\S+)`)
	
	// Email Security Appliance (ESA) MID Tracking format:
	// Info: MID 12345 ICID 67890 
	p.esaRe = regexp.MustCompile(`(?i)Info:.*?(MID (?P<mid>\d+)|ICID (?P<icid>\d+)|DCID (?P<dcid>\d+))`)
	return nil
}

func (p *IronportPlugin) Process(entry *parsers.LogEntry) {
	if strings.Contains(entry.Line, "TCP_MISS") || strings.Contains(entry.Line, "TCP_HIT") || strings.Contains(entry.Line, "TCP_DENIED") {
		matches := p.wsaRe.FindStringSubmatch(entry.Line)
		if len(matches) > 0 {
			if entry.ParsedFields == nil {
				entry.ParsedFields = make(map[string]string)
			}
			entry.AppID = "ironport-wsa"
			names := p.wsaRe.SubexpNames()
			for i, match := range matches {
				if names[i] != "" && match != "" {
					entry.ParsedFields[names[i]] = match
				}
			}
			return
		}
	}

	if strings.Contains(entry.Line, "Info: MID") || strings.Contains(entry.Line, "ICID") {
		matches := p.esaRe.FindAllStringSubmatch(entry.Line, -1)
		if len(matches) > 0 {
			if entry.ParsedFields == nil {
				entry.ParsedFields = make(map[string]string)
			}
			entry.AppID = "ironport-esa"
			
			// This regex is iterative so we process all hits
			for _, m := range matches {
				for i, name := range p.esaRe.SubexpNames() {
					if name != "" && m[i] != "" {
						entry.ParsedFields[name] = m[i]
					}
				}
			}
		}
	}
}
