package search

import (
	"afterloggrator/parsers"
	"context"
	"fmt"
	"go.starlark.net/starlark"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

// Script runs pure, frozen Starlark modules with an instruction budget per call.
// It exposes no process, network, or filesystem builtins; load is confined to the script directory.
type Script struct {
	mu  sync.Mutex
	fn  starlark.Callable
	ctx context.Context
}

func LoadScript(ctx context.Context, path string) (*Script, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	cache := map[string]starlark.StringDict{}
	loading := map[string]bool{}
	predeclared := starlark.StringDict{"regex_match": starlark.NewBuiltin("regex_match", func(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var pattern, text string
		if err := starlark.UnpackArgs(b.Name(), args, kwargs, "pattern", &pattern, "text", &text); err != nil {
			return nil, err
		}
		ok, err := regexp.MatchString(pattern, text)
		return starlark.Bool(ok), err
	})}
	var load func(*starlark.Thread, string) (starlark.StringDict, error)
	load = func(_ *starlark.Thread, name string) (starlark.StringDict, error) {
		name = filepath.Clean(name)
		if !filepath.IsLocal(name) {
			return nil, fmt.Errorf("module must stay inside the script directory")
		}
		if g, ok := cache[name]; ok {
			return g, nil
		}
		if loading[name] {
			return nil, fmt.Errorf("cyclic load: %s", name)
		}
		if len(cache)+len(loading) >= 64 {
			return nil, fmt.Errorf("module limit exceeded")
		}
		loading[name] = true
		defer delete(loading, name)
		f, err := root.Open(name)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
		f.Close()
		if err != nil {
			return nil, err
		}
		if len(data) > 1<<20 {
			return nil, fmt.Errorf("script exceeds 1 MiB")
		}
		thread := newThread(ctx)
		thread.Load = load
		stop := context.AfterFunc(ctx, func() { thread.Cancel("context canceled") })
		defer stop()
		g, err := starlark.ExecFile(thread, name, data, predeclared)
		if err != nil {
			return nil, err
		}
		g.Freeze()
		cache[name] = g
		return g, nil
	}
	globals, err := load(nil, filepath.Base(path))
	if err != nil {
		return nil, err
	}
	fn, ok := globals["filter"].(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("script must define filter(event) returning bool")
	}
	return &Script{fn: fn, ctx: ctx}, nil
}
func newThread(ctx context.Context) *starlark.Thread {
	t := &starlark.Thread{Name: "filter", Print: func(_ *starlark.Thread, msg string) { fmt.Fprintln(os.Stderr, msg) }}
	t.SetMaxExecutionSteps(100000)
	if ctx.Err() != nil {
		t.Cancel("context canceled")
	}
	return t
}
func (s *Script) Match(e parsers.LogEntry) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := starlark.NewDict(12)
	for _, key := range []string{"source", "host", "service", "container", "cluster", "app_id", "line", "filename"} {
		d.SetKey(starlark.String(key), starlark.String(e.Field(key)))
	}
	ts := ""
	if !e.Timestamp.IsZero() {
		ts = e.Timestamp.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	}
	d.SetKey(starlark.String("timestamp"), starlark.String(ts))
	fields := starlark.NewDict(len(e.ParsedFields))
	for k, v := range e.ParsedFields {
		fields.SetKey(starlark.String(k), starlark.String(v))
	}
	d.SetKey(starlark.String("fields"), fields)
	tokens := make([]starlark.Value, len(e.Tokens))
	for i, t := range e.Tokens {
		tokens[i] = starlark.String(t)
	}
	d.SetKey(starlark.String("tokens"), starlark.NewList(tokens))
	d.Freeze()
	thread := newThread(s.ctx)
	stop := context.AfterFunc(s.ctx, func() { thread.Cancel("context canceled") })
	defer stop()
	result, err := starlark.Call(thread, s.fn, starlark.Tuple{d}, nil)
	if err != nil {
		return false, err
	}
	b, ok := result.(starlark.Bool)
	if !ok {
		return false, fmt.Errorf("filter(event) returned %s; expected bool", result.Type())
	}
	return bool(b), nil
}
