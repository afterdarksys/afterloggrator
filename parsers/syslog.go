package parsers

import (
	"encoding/json"
	"os"
	"regexp"

	"github.com/dlclark/regexp2"
)

type SyslogEngine struct {
	ActiveRules []ParsedRule
}

type ParsedRule struct {
	AppID string
	Regex *regexp2.Regexp
}

var isNumeric = regexp.MustCompile(`^\d+$`)

func NewSyslogEngine(rulesFile string) *SyslogEngine {
	engine := &SyslogEngine{}

	defaultRules := map[string]string{
		"sshd":     `(?i)(Accepted (publickey|password) for (?<user>\S+) from (?<ip>[0-9\.]+)|Failed password.*ssh|Invalid user.*ssh|sshd\[\d+\]:)`,
		"iptables": `(?i)(kernel: \[.*\] .*IN=(?<in_iface>\S+).*OUT=(?<out_iface>\S*).*SRC=(?<src_ip>\S+).*DST=(?<dst_ip>\S+).*SPT=(?<src_port>\d+).*DPT=(?<dst_port>\d+))`,
		"nginx":    `(?i)(nginx)`,
		"apache":   `(?i)(apache2?|httpd)`,
		"syslog":   `(?i)(systemd\[\d+\]:|CRON\[\d+\]:|syslogd)`,
	}

	if rulesFile != "" {
		b, err := os.ReadFile(rulesFile)
		if err == nil {
			var config struct {
				Rules []struct {
					AppID string `json:"app"`
					Match string `json:"match"`
				} `json:"rules"`
			}
			if err := json.Unmarshal(b, &config); err == nil && len(config.Rules) > 0 {
				for _, r := range config.Rules {
					re, err := regexp2.Compile(r.Match, regexp2.None)
					if err == nil {
						engine.ActiveRules = append(engine.ActiveRules, ParsedRule{AppID: r.AppID, Regex: re})
					}
				}
				return engine
			}
		}
	}

	for app, m := range defaultRules {
		re, _ := regexp2.Compile(m, regexp2.None)
		engine.ActiveRules = append(engine.ActiveRules, ParsedRule{AppID: app, Regex: re})
	}
	return engine
}

func (e *SyslogEngine) Parse(line string) (string, map[string]string) {
	for _, rule := range e.ActiveRules {
		m, _ := rule.Regex.FindStringMatch(line)
		if m != nil {
			parsed := make(map[string]string)
			for _, g := range m.Groups() {
				if g.Name != "" && !isNumeric.MatchString(g.Name) {
					if len(g.Captures) > 0 {
						parsed[g.Name] = g.String()
					}
				}
			}
			return rule.AppID, parsed
		}
	}
	return "", nil
}
