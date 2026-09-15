package main

import (
	"afterloggrator/parsers"
	"afterloggrator/search"
	"afterloggrator/sources"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/dlclark/regexp2"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type stringsFlag []string

func (s *stringsFlag) String() string     { return strings.Join(*s, ",") }
func (s *stringsFlag) Set(v string) error { *s = append(*s, v); return nil }
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "afterloggrator:", err)
		os.Exit(1)
	}
}
func run(ctx context.Context, args []string, out, stderr io.Writer) error {
	fs := flag.NewFlagSet("afterloggrator", flag.ContinueOnError)
	fs.SetOutput(stderr)
	config := fs.String("sources", "", "JSON source configuration")
	follow := fs.Bool("f", false, "Follow files, SSH, Docker and Kubernetes logs")
	recursive := fs.Bool("d", false, "Read files recursively from directories")
	sys := fs.Bool("sys", false, "Include local system log paths")
	pattern := fs.String("filter", "", "regexp2 line pattern")
	ignoreCase := fs.Bool("i", false, "Case insensitive -filter")
	app := fs.String("app", "", "Exact parsed application filter")
	sinceStr := fs.String("since", "", "Inclusive start event time (RFC3339)")
	untilStr := fs.String("until", "", "Inclusive end event time (RFC3339)")
	script := fs.String("script", "", "Starlark file defining filter(event) -> bool")
	correlate := fs.Bool("correlate", false, "Correlate matching IP/MAC/domain tokens")
	corrFields := fs.String("correlate-by", "", "Comma-separated fields; use request_id=trace_id for aliases")
	window := fs.Duration("window", 5*time.Minute, "Maximum event-time gap for correlation; 0 is unlimited")
	buffer := fs.Int("buffer", 5000, "Maximum retained events for correlation")
	sorted := fs.Bool("sort", false, "Collate finite results by event timestamp")
	maxResults := fs.Int("max-results", 100000, "Maximum events buffered by -sort; exceeding it fails")
	concurrency := fs.Int("concurrency", 8, "Maximum concurrent sources")
	jsonOutput := fs.Bool("json", false, "Output normalized JSON lines")
	timestamp := fs.Bool("t", false, "Show event timestamps")
	fs.Bool("no-color", false, "Compatibility flag; output is always plain text")
	sshFlag := fs.Bool("ssh", false, "Read positional paths from each -hosts SSH target")
	hosts := fs.String("hosts", "", "Comma-separated SSH hosts")
	user := fs.String("ssh-user", os.Getenv("USER"), "SSH user")
	home, _ := os.UserHomeDir()
	key := fs.String("ssh-key", filepath.Join(home, ".ssh", "id_rsa"), "SSH private key")
	known := fs.String("known-hosts", filepath.Join(home, ".ssh", "known_hosts"), "SSH known_hosts file")
	var contains, where stringsFlag
	fs.Var(&contains, "contains", "Required literal string; repeatable (AND)")
	fs.Var(&where, "where", "Exact field=value condition; repeatable (AND)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	since, err := search.ParseTime(*sinceStr)
	if err != nil {
		return err
	}
	until, err := search.ParseTime(*untilStr)
	if err != nil {
		return err
	}
	if !since.IsZero() && !until.IsZero() && since.After(until) {
		return fmt.Errorf("since must not be after until")
	}
	if *buffer <= 0 || *maxResults <= 0 || *concurrency <= 0 || *window < 0 {
		return fmt.Errorf("buffer, max-results and concurrency must be positive; window must be nonnegative")
	}
	if *follow && *sorted {
		return fmt.Errorf("-sort requires a finite search")
	}
	q := search.Query{Contains: contains, Fields: map[string]string{}, Since: since, Until: until}
	for _, v := range where {
		k, val, ok := strings.Cut(v, "=")
		if !ok || k == "" {
			return fmt.Errorf("invalid -where %q", v)
		}
		if _, exists := q.Fields[k]; exists {
			return fmt.Errorf("duplicate field %q", k)
		}
		q.Fields[k] = val
	}
	if *app != "" {
		q.Fields["app_id"] = *app
	}
	if *pattern != "" {
		options := regexp2.None
		if *ignoreCase {
			options = regexp2.IgnoreCase
		}
		q.Regex, err = regexp2.Compile(*pattern, options)
		if err != nil {
			return err
		}
		q.Regex.MatchTimeout = 100 * time.Millisecond
	}
	if *script != "" {
		q.Script, err = search.LoadScript(ctx, *script)
		if err != nil {
			return err
		}
	}
	var list []sources.Source
	if *config != "" {
		list, err = sources.Load(*config)
		if err != nil {
			return err
		}
	}
	files := fs.Args()
	if *sys {
		files = append(files, parsers.GetSystemLogPaths()...)
	}
	if *sshFlag && (*hosts == "" || len(files) == 0) {
		return fmt.Errorf("-ssh requires -hosts and paths")
	}
	if !*sshFlag && *hosts != "" {
		return fmt.Errorf("-hosts requires -ssh")
	}
	if *sshFlag && (*recursive || *sys) {
		return fmt.Errorf("-ssh cannot combine with -d or -sys")
	}
	if *recursive {
		var expanded []string
		for _, p := range files {
			if err = filepath.WalkDir(p, func(path string, d os.DirEntry, e error) error {
				if e != nil {
					return e
				}
				if d.Type().IsRegular() {
					expanded = append(expanded, path)
				}
				return nil
			}); err != nil {
				return err
			}
		}
		files = expanded
	}
	for _, p := range files {
		if *sshFlag {
			for _, h := range strings.Split(*hosts, ",") {
				h = strings.TrimSpace(h)
				if h == "" {
					return fmt.Errorf("empty SSH host")
				}
				list = append(list, sources.Source{Name: h + ":" + p, Type: "ssh", Path: p, Host: h, User: *user, KeyFile: *key, KnownHosts: *known})
			}
		} else {
			list = append(list, sources.Source{Name: p, Type: "file", Path: p})
		}
	}
	if *follow && len(list) > *concurrency {
		return fmt.Errorf("follow needs -concurrency at least %d so every source can start", len(list))
	}
	if len(list) == 0 {
		return fmt.Errorf("specify files or -sources")
	}
	if err = sources.Validate(list); err != nil {
		return err
	}
	for _, s := range list {
		if *follow && s.Type != "file" && s.Type != "ssh" && s.Type != "docker" && s.Type != "kubernetes" {
			return fmt.Errorf("source %s: %s supports finite searches only", s.Name, s.Type)
		}
		if s.Type == "oci" && (since.IsZero() || until.IsZero()) {
			return fmt.Errorf("OCI sources require -since and -until")
		}
	}
	engines := []parsers.Engine{parsers.NewAWSEngine(), parsers.NewAzureEngine(), parsers.NewGCPEngine(), parsers.NewOCIEngine(), parsers.NewCloudflareEngine(), parsers.NewDockerEngine(), parsers.NewSyslogEngine("")}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	entries := make(chan parsers.LogEntry, 256)
	jobs := make(chan sources.Source)
	errs := make(chan error, len(list))
	var wg sync.WaitGroup
	n := *concurrency
	if n > len(list) {
		n = len(list)
	}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for s := range jobs {
				e := sources.Read(ctx, s, sources.Options{Follow: *follow, Since: since, Until: until}, func(e parsers.LogEntry) error {
					select {
					case entries <- e:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				})
				if e != nil && ctx.Err() == nil {
					errs <- fmt.Errorf("source %s: %w", s.Name, e)
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, s := range list {
			select {
			case jobs <- s:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { wg.Wait(); close(entries); close(errs) }()
	c := search.Correlator{Limit: *buffer, Window: *window, Tokens: *correlate}
	if *corrFields != "" {
		for _, field := range strings.Split(*corrFields, ",") {
			field = strings.TrimSpace(field)
			if field == "" {
				cancel()
				for range entries {
				}
				return fmt.Errorf("empty correlation field")
			}
			c.Fields = append(c.Fields, field)
		}
	}
	encoder := json.NewEncoder(out)
	output := func(e parsers.LogEntry) error {
		if *jsonOutput {
			return encoder.Encode(e)
		}
		prefix := ""
		if e.Correlated {
			prefix = "[LINKED] "
		}
		if *timestamp {
			t := "unknown"
			if !e.Timestamp.IsZero() {
				t = e.Timestamp.Format(time.RFC3339Nano)
			}
			prefix += "[" + t + "] "
		}
		_, err := fmt.Fprintf(out, "%s[%s] %s\n", prefix, e.Source, e.Line)
		return err
	}
	var results []parsers.LogEntry
	var processingErr error
	for e := range entries {
		if processingErr != nil {
			continue
		}
		parsers.Normalize(&e, engines)
		if !q.InRange(e) {
			continue
		}
		match, eErr := q.Match(e)
		if eErr != nil {
			processingErr = eErr
			cancel()
			continue
		}
		var selected []parsers.LogEntry
		if *correlate || len(c.Fields) > 0 {
			selected = c.Add(e, match)
		} else if match {
			selected = []parsers.LogEntry{e}
		}
		for _, record := range selected {
			if *sorted {
				if len(results) >= *maxResults {
					processingErr = fmt.Errorf("sort exceeds -max-results=%d; narrow search or increase limit", *maxResults)
					cancel()
					break
				}
				results = append(results, record)
			} else if err = output(record); err != nil {
				processingErr = err
				cancel()
				break
			}
		}
	}
	var failures []error
	if processingErr != nil {
		failures = append(failures, processingErr)
	}
	for e := range errs {
		failures = append(failures, e)
	}
	if ctx.Err() != nil && processingErr == nil {
		failures = append(failures, ctx.Err())
	}
	if len(failures) > 0 {
		return errors.Join(failures...)
	}
	if *sorted {
		search.Sort(results)
		for _, e := range results {
			if err = output(e); err != nil {
				return err
			}
		}
	}
	return nil
}
