package plugins

import (
	"afterloggrator/parsers"
)

type Plugin interface {
	Init() error
	Process(entry *parsers.LogEntry)
}

type DiagnosticsPlugin interface {
	RunDiagnostics(collectedLogs []string)
}

var ActivePlugins []Plugin
var ActiveDiagnostics []DiagnosticsPlugin

func Dispatch(entry *parsers.LogEntry) {
	for _, p := range ActivePlugins {
		p.Process(entry)
	}
}

func RunDiagnostics(collectedLogs []string) {
	for _, p := range ActiveDiagnostics {
		p.RunDiagnostics(collectedLogs)
	}
}
