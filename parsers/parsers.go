package parsers

import "time"

const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorGray   = "\033[37m"
)

type LogEntry struct {
	Filename     string            `json:"filename"`
	Line         string            `json:"line"`
	Timestamp    time.Time         `json:"timestamp"`
	Color        string            `json:"-"`
	AppID        string            `json:"app_id"`
	Printed      bool              `json:"-"`
	ParsedFields map[string]string `json:"parsed_fields,omitempty"`
	Tokens       []string          `json:"tokens,omitempty"`
}

type Engine interface {
	Parse(line string) (string, map[string]string)
}
