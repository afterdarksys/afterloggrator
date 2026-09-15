package plugins

import (
	"afterloggrator/parsers"
	"fmt"
)

// StarlarkPlugin is a migration shim. The old process_log interface silently
// ignored return values and exposed unrestricted I/O. Use search.LoadScript.
type StarlarkPlugin struct{ Dir string }

func (s *StarlarkPlugin) SetParsers([]parsers.Engine)                   {}
func (s *StarlarkPlugin) SetCorrelator(func(string) []parsers.LogEntry) {}
func (s *StarlarkPlugin) Init() error {
	return fmt.Errorf("legacy Starlark plugins are retired; use -script with filter(event) returning bool")
}
func (s *StarlarkPlugin) Process(*parsers.LogEntry) {}
