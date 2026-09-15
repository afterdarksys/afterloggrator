package plugins

import (
	"afterloggrator/parsers"
	"fmt"
)

// SubprocessPlugin is retained only as a migration shim. Automatic executable
// discovery and unbounded asynchronous stdin writes are no longer supported.
type SubprocessPlugin struct{ Dir string }
type PluginResponse struct {
	Plugin string `json:"plugin"`
	Msg    string `json:"msg"`
	Color  string `json:"color"`
}

func (s *SubprocessPlugin) Init() error {
	return fmt.Errorf("executable plugin discovery is retired; enrich upstream and use -script for filters")
}
func (s *SubprocessPlugin) Process(*parsers.LogEntry) {}
