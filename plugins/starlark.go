package plugins

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	"afterloggrator/parsers"

	"go.starlark.net/starlark"
)

type StarlarkPlugin struct {
	Dir     string
	scripts map[string]*starlark.Thread
	globals map[string]starlark.StringDict
}

var starlarkParsers []parsers.Engine
var starlarkCorrelateCallback func(token string) []parsers.LogEntry

func (s *StarlarkPlugin) SetParsers(engines []parsers.Engine) {
	starlarkParsers = engines
}

func (s *StarlarkPlugin) SetCorrelator(cb func(string) []parsers.LogEntry) {
	starlarkCorrelateCallback = cb
}

func (s *StarlarkPlugin) Init() error {
	s.scripts = make(map[string]*starlark.Thread)
	s.globals = make(map[string]starlark.StringDict)

	if _, err := os.Stat(s.Dir); os.IsNotExist(err) {
		return nil
	}

	files, err := os.ReadDir(s.Dir)
	if err != nil {
		return err
	}

	for _, f := range files {
		if filepath.Ext(f.Name()) == ".star" {
			path := filepath.Join(s.Dir, f.Name())
			thread := &starlark.Thread{Name: f.Name()}

			builtins := starlark.StringDict{
				"resolve_ip":   starlark.NewBuiltin("resolve_ip", starlarkResolveIP),
				"resolve_name": starlark.NewBuiltin("resolve_name", starlarkResolveName),
				"check_rbl":    starlark.NewBuiltin("check_rbl", starlarkCheckRBL),
				"read_file":    starlark.NewBuiltin("read_file", starlarkReadFile),
				"read_lines":   starlark.NewBuiltin("read_lines", starlarkReadLines),
				"write_file":   starlark.NewBuiltin("write_file", starlarkWriteFile),
				"append_file":  starlark.NewBuiltin("append_file", starlarkAppendFile),
				"parse_log":    starlark.NewBuiltin("parse_log", starlarkParseLog),
				"correlate":    starlark.NewBuiltin("correlate", starlarkCorrelate),
				"http_get":     starlark.NewBuiltin("http_get", starlarkHTTPGet),
				"http_post":    starlark.NewBuiltin("http_post", starlarkHTTPPost),
				"exec":         starlark.NewBuiltin("exec", starlarkExec),
				"regex_match":  starlark.NewBuiltin("regex_match", starlarkRegexMatch),
			}

			globals, err := starlark.ExecFile(thread, path, nil, builtins)
			if err != nil {
				fmt.Printf("Error loading starlark script %s: %v\n", f.Name(), err)
				continue
			}
			s.scripts[f.Name()] = thread
			s.globals[f.Name()] = globals
		}
	}
	return nil
}

// -- LogScript Built-ins --

func starlarkResolveIP(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var ip string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "ip", &ip); err != nil {
		return nil, err
	}
	names, err := net.LookupAddr(ip)
	if err != nil || len(names) == 0 {
		return starlark.String(""), nil
	}
	return starlark.String(names[0]), nil
}

func starlarkResolveName(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	ips, err := net.LookupHost(name)
	if err != nil || len(ips) == 0 {
		return starlark.String(""), nil
	}
	return starlark.String(ips[0]), nil
}

func starlarkCheckRBL(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var ip, rbl string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "ip", &ip, "rbl", &rbl); err != nil {
		return nil, err
	}
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil || parsedIP.To4() == nil {
		return starlark.False, nil
	}
	parsedIP = parsedIP.To4()
	query := fmt.Sprintf("%d.%d.%d.%d.%s", parsedIP[3], parsedIP[2], parsedIP[1], parsedIP[0], rbl)
	_, err := net.LookupHost(query)
	if err != nil {
		return starlark.False, nil
	}
	return starlark.True, nil
}

func starlarkReadFile(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "path", &path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return starlark.String(""), nil
	}
	return starlark.String(string(data)), nil
}

func starlarkWriteFile(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path, data string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "path", &path, "data", &data); err != nil {
		return nil, err
	}
	err := os.WriteFile(path, []byte(data), 0644)
	if err != nil {
		return starlark.False, nil
	}
	return starlark.True, nil
}

func starlarkAppendFile(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path, data string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "path", &path, "data", &data); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return starlark.False, nil
	}
	defer f.Close()
	if _, err := f.WriteString(data); err != nil {
		return starlark.False, nil
	}
	return starlark.True, nil
}

func starlarkExec(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var cmd string
	var cmdArgs *starlark.List
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "cmd", &cmd, "args?", &cmdArgs); err != nil {
		return nil, err
	}
	
	var execArgs []string
	if cmdArgs != nil {
		for i := 0; i < cmdArgs.Len(); i++ {
			if str, ok := cmdArgs.Index(i).(starlark.String); ok {
				execArgs = append(execArgs, string(str))
			}
		}
	}
	
	out, err := exec.Command(cmd, execArgs...).CombinedOutput()
	if err != nil {
		return starlark.String(""), nil // Silently fail externally, or we could pass err strings
	}
	return starlark.String(string(out)), nil
}

func starlarkRegexMatch(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var pattern, text string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "pattern", &pattern, "text", &text); err != nil {
		return nil, err
	}
	matched, err := regexp.MatchString(pattern, text)
	if err != nil || !matched {
		return starlark.False, nil
	}
	return starlark.True, nil
}

func starlarkHTTPGet(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var url string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "url", &url); err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return starlark.String(""), nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return starlark.String(string(body)), nil
}

func starlarkHTTPPost(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var url, bodyStr, contentType string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "url", &url, "body", &bodyStr, "content_type", &contentType); err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(url, contentType, bytes.NewBuffer([]byte(bodyStr)))
	if err != nil {
		return starlark.String(""), nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return starlark.String(string(body)), nil
}

func starlarkReadLines(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "path", &path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return starlark.NewList(nil), nil
	}
    list := starlark.NewList(nil)
    var current string
    for _, c := range data {
        if c == '\n' {
            list.Append(starlark.String(current))
            current = ""
        } else {
            current += string(c)
        }
    }
    if len(current) > 0 {
		list.Append(starlark.String(current))
	}
	return list, nil
}

func starlarkParseLog(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var line string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "line", &line); err != nil {
		return nil, err
	}
	
	appID := ""
	var parsedFields map[string]string
	for _, engine := range starlarkParsers {
		app, parsed := engine.Parse(line)
		if app != "" {
			appID = app
			parsedFields = parsed
			break
		}
	}
	
	dict := starlark.NewDict(0)
	dict.SetKey(starlark.String("app_id"), starlark.String(appID))
	
	fieldsDict := starlark.NewDict(0)
	for k, v := range parsedFields {
		fieldsDict.SetKey(starlark.String(k), starlark.String(v))
	}
	dict.SetKey(starlark.String("fields"), fieldsDict)

	tokensList := starlark.NewList(nil)
	for _, t := range parsers.FindTokensFast(line) {
		tokensList.Append(starlark.String(t))
	}
	dict.SetKey(starlark.String("tokens"), tokensList)

	return dict, nil
}

func starlarkCorrelate(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var token string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "token", &token); err != nil {
		return nil, err
	}
	
	var entries []parsers.LogEntry
	if starlarkCorrelateCallback != nil {
		entries = starlarkCorrelateCallback(token)
	}
	
	list := starlark.NewList(nil)
	for _, e := range entries {
		dict := starlark.NewDict(0)
		dict.SetKey(starlark.String("line"), starlark.String(e.Line))
		dict.SetKey(starlark.String("app_id"), starlark.String(e.AppID))
		list.Append(dict)
	}
	return list, nil
}

func (s *StarlarkPlugin) Process(entry *parsers.LogEntry) {
	for name, globals := range s.globals {
		if processFunc, ok := globals["process_log"]; ok {
			thread := s.scripts[name]
			
			// Native mapping of Go struct to starlark dictionary for execution
			dict := starlark.NewDict(0)
			dict.SetKey(starlark.String("line"), starlark.String(entry.Line))
			dict.SetKey(starlark.String("app_id"), starlark.String(entry.AppID))
			
			tokensList := starlark.NewList(nil)
			for _, t := range entry.Tokens {
				tokensList.Append(starlark.String(t))
			}
			dict.SetKey(starlark.String("tokens"), tokensList)
			
			_, err := starlark.Call(thread, processFunc, starlark.Tuple{dict}, nil)
			if err != nil {
				// Suppress errors dynamically in logs but they can be traced
			}
		}
	}
}
