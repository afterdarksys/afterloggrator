package plugins

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"afterloggrator/parsers"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type SubprocessPlugin struct {
	Dir       string
	processes []*struct {
		Name  string
		Stdin io.WriteCloser
	}
}

type PluginResponse struct {
	Plugin string `json:"plugin"`
	Msg    string `json:"msg"`
	Color  string `json:"color"`
}

func (s *SubprocessPlugin) Init() error {
	if _, err := os.Stat(s.Dir); os.IsNotExist(err) {
		return nil
	}

	files, err := os.ReadDir(s.Dir)
	if err != nil {
		return err
	}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		path := filepath.Join(s.Dir, f.Name())
		info, err := os.Stat(path)
		if err != nil || info.Mode()&0111 == 0 {
			continue
		}

		cmd := exec.Command(path)
		stdin, err := cmd.StdinPipe()
		if err != nil {
			continue
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			continue
		}

		if err := cmd.Start(); err != nil {
			continue
		}

		s.processes = append(s.processes, &struct {
			Name  string
			Stdin io.WriteCloser
		}{Name: f.Name(), Stdin: stdin})

		go func(pluginName string, reader io.Reader) {
			scanner := bufio.NewScanner(reader)
			for scanner.Scan() {
				var resp PluginResponse
				if err := json.Unmarshal(scanner.Bytes(), &resp); err == nil {
					colorCode := parsers.ColorCyan
					switch strings.ToLower(resp.Color) {
					case "red":
						colorCode = parsers.ColorRed
					case "yellow":
						colorCode = parsers.ColorYellow
					case "green":
						colorCode = parsers.ColorGreen
					case "purple":
						colorCode = parsers.ColorPurple
					}
					pluginIdentifier := resp.Plugin
					if pluginIdentifier == "" {
						pluginIdentifier = pluginName
					}
					fmt.Printf("%s↳ [%s] %s%s\n", colorCode, pluginIdentifier, resp.Msg, parsers.ColorReset)
				}
			}
		}(f.Name(), stdout)
	}
	return nil
}

func (s *SubprocessPlugin) Process(entry *parsers.LogEntry) {
	if len(s.processes) == 0 {
		return
	}
	b, err := json.Marshal(entry)
	if err == nil {
		b = append(b, '\n')
		for _, p := range s.processes {
			// CRITICAL PERFORMANCE FIX: Execute plugin I/O asynchronously.
			// This guarantees that a slow Python ML scoring script won't bottleneck the high-speed main loop processing 100k+ EPS.
			go func(pipe io.WriteCloser, data []byte) {
				pipe.Write(data)
			}(p.Stdin, b)
		}
	}
}
