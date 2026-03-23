package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"afterloggrator/extractors"
	"afterloggrator/parsers"
	"afterloggrator/plugins"

	"github.com/dlclark/regexp2"
)

var printMu sync.Mutex

func safePrint(format string, args ...interface{}) {
	printMu.Lock()
	defer printMu.Unlock()
	fmt.Printf(format, args...)
}

func printEntry(entry parsers.LogEntry, showFile, showTimestamp, noColor, isCorrelated bool) {
	prefix := ""
	if showFile {
		prefix = fmt.Sprintf("%s[%s]%s ", entry.Color, filepath.Base(entry.Filename), parsers.ColorReset)
	}

	tsPrefix := ""
	if showTimestamp {
		tsPrefix = fmt.Sprintf("[%s] ", entry.Timestamp.Format("15:04:05"))
	}

	appPrefix := ""
	if entry.AppID != "" {
		appPrefix = fmt.Sprintf("{%s} ", entry.AppID)
	}

	corrPrefix := ""
	if isCorrelated {
		corrPrefix = fmt.Sprintf("%s[LINKED]%s ", parsers.ColorPurple, parsers.ColorReset)
	}

	line := entry.Line
	if !noColor {
		line = highlightKeywords(line)
	}

	if len(entry.ParsedFields) > 0 {
		var fields []string
		for k, v := range entry.ParsedFields {
			fields = append(fields, fmt.Sprintf("%s=%s", k, v))
		}
		if noColor {
			line += fmt.Sprintf(" {ext: %s}", strings.Join(fields, ", "))
		} else {
			line += fmt.Sprintf(" %s{ext: %s}%s", parsers.ColorGray, strings.Join(fields, ", "), parsers.ColorReset)
		}
	}

	safePrint("%s%s%s%s%s\n", corrPrefix, tsPrefix, prefix, appPrefix, line)
}

func highlightKeywords(line string) string {
	keywords := map[string]string{
		"ERROR":   parsers.ColorRed,
		"FATAL":   parsers.ColorRed,
		"WARN":    parsers.ColorYellow,
		"WARNING": parsers.ColorYellow,
		"INFO":    parsers.ColorGreen,
		"DEBUG":   parsers.ColorGray,
	}

	result := line
	for keyword, color := range keywords {
		result = strings.ReplaceAll(result, keyword, color+keyword+parsers.ColorReset)
	}
	return result
}

func catFile(filename, color string, logChan chan<- parsers.LogEntry) {
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()

	reader, err := extractors.GetReader(file, filename)
	if err != nil {
		return
	}
	if reader != file {
		defer reader.Close()
	}

	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		logChan <- parsers.LogEntry{
			Filename:  filename,
			Line:      scanner.Text(),
			Timestamp: time.Now(),
			Color:     color,
		}
	}
}

func tailFile(ctx context.Context, filename, color string, logChan chan<- parsers.LogEntry) {
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(filename))
	if ext == ".gz" || ext == ".bz2" || ext == ".z" || ext == ".zip" || ext == ".tar" {
		catFile(filename, color, logChan) 
		return
	}

	file.Seek(0, os.SEEK_END)
	reader := bufio.NewReader(file)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		
		line, err := reader.ReadString('\n')
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		logChan <- parsers.LogEntry{
			Filename:  filename,
			Line:      strings.TrimRight(line, "\n"),
			Timestamp: time.Now(),
			Color:     color,
		}
	}
}

var (
	ipRegex  = regexp2.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`, regexp2.None)
	macRegex = regexp2.MustCompile(`\b(?:[0-9A-Fa-f]{2}[:-]){5}(?:[0-9A-Fa-f]{2})\b`, regexp2.None)
	fileColors = []string{
		parsers.ColorCyan,
		parsers.ColorGreen,
		parsers.ColorYellow,
		parsers.ColorBlue,
		parsers.ColorPurple,
	}
)

func findTokens(regex *regexp2.Regexp, line string) []string {
	var tokens []string
	m, _ := regex.FindStringMatch(line)
	for m != nil {
		tokens = append(tokens, m.String())
		m, _ = regex.FindNextMatch(m)
	}
	return tokens
}

type WorkerResult struct {
	Entry          parsers.LogEntry
	IsPrimaryMatch bool
	LineTokens     []string
}

func loadConfig() {
	cliFlags := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		cliFlags[f.Name] = true
	})

	home, _ := os.UserHomeDir()
	paths := []string{
		"/etc/afterlogger/config.cfg",
		filepath.Join(home, ".afterlogger", "config.cfg"),
	}

	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					k := strings.TrimSpace(parts[0])
					v := strings.TrimSpace(parts[1])
					v = strings.Trim(v, `"'`)
					if !cliFlags[k] && flag.Lookup(k) != nil {
						flag.Set(k, v)
					}
				}
			}
		}
	}
}

func main() {
	follow := flag.Bool("f", false, "Follow log files (like tail -f)")
	filterStr := flag.String("filter", "", "Filter lines matching PCRE regex pattern")
	ignoreCase := flag.Bool("i", false, "Case-insensitive filtering")
	timestampFlag := flag.Bool("t", false, "Add timestamp to each line")
	noColor := flag.Bool("no-color", false, "Disable colored output")
	dirScan := flag.Bool("d", false, "Scan directories recursively for log files")
	appFilter := flag.String("app", "", "Filter by application (e.g., sshd, iptables, nginx)")
	correlate := flag.Bool("correlate", false, "Automatically extract IPs/domains to link events")
	useRegex := flag.Bool("use-regex", false, "Use PCRE regexp2 engine for IP/MAC extraction instead of the high-performance tokenizer")
	sysScan := flag.Bool("sys", false, "Automatically include host system logs (/var/log/syslog, etc)")
	claude := flag.Bool("claude", false, "Use Claude to automatically diagnose the matched logs (requires ANTHROPIC_API_KEY)")
	pluginsDir := flag.String("plugins-dir", "plugins", "Directory containing executable plugins")
	rulesFile := flag.String("rules", "", "Path to custom rules.json parsing configurations")

	sshFlag := flag.Bool("ssh", false, "Use SSH to pull logs natively from remote clusters")
	hostsFlag := flag.String("hosts", "", "Comma-separated list of remote hostnames or IPs for distributed logging")
	sshUser := flag.String("ssh-user", os.Getenv("USER"), "The remote SSH user account")
	homeDir, _ := os.UserHomeDir()
	sshKey := flag.String("ssh-key", filepath.Join(homeDir, ".ssh", "id_rsa"), "The absolute path to the local RSA/Ed25519 identity file")

	var esURL, esIndex, esUser, esPass string
	flag.StringVar(&esURL, "es-url", "", "Elasticsearch URL (e.g., http://localhost:9200)")
	flag.StringVar(&esIndex, "es-index", "*", "Elasticsearch index to search")
	flag.StringVar(&esUser, "es-user", "", "Elasticsearch username")
	flag.StringVar(&esPass, "es-pass", "", "Elasticsearch password")

	flag.Parse()
	loadConfig()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: No log files or directories specified")
		os.Exit(1)
	}

	var files []string
	if *dirScan {
		for _, arg := range args {
			filepath.Walk(arg, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return nil
				}
				if !info.IsDir() {
					files = append(files, path)
				}
				return nil
			})
		}
	} else {
		files = args
	}

	if *sysScan {
		files = append(files, parsers.GetSystemLogPaths()...)
	}

	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "Error: No files found to monitor")
		os.Exit(1)
	}

	var filterRegex *regexp2.Regexp
	if *filterStr != "" {
		pattern := *filterStr
		if *ignoreCase {
			pattern = "(?i)" + pattern
		}
		var err error
		filterRegex, err = regexp2.Compile(pattern, regexp2.None)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid PCRE regex pattern: %v\n", err)
			os.Exit(1)
		}
	}

	activeParsers := []parsers.Engine{
		parsers.NewSyslogEngine(*rulesFile),
		parsers.NewAWSEngine(),
		parsers.NewAzureEngine(),
		parsers.NewGCPEngine(),
		parsers.NewOCIEngine(),
		parsers.NewCloudflareEngine(),
		parsers.NewDockerEngine(),
	}

	plugins.ActivePlugins = append(plugins.ActivePlugins, &plugins.PaloAltoPlugin{})
	plugins.ActivePlugins = append(plugins.ActivePlugins, &plugins.ProofpointPlugin{})
	plugins.ActivePlugins = append(plugins.ActivePlugins, &plugins.IronportPlugin{})

	mlPlugin := &plugins.SubprocessPlugin{Dir: *pluginsDir}
	plugins.ActivePlugins = append(plugins.ActivePlugins, mlPlugin)

	var ringMu sync.Mutex
	var ringBuffer []*parsers.LogEntry
	ringSize := 5000
	invertedIndex := make(map[string][]*parsers.LogEntry)

	starlarkPlugin := &plugins.StarlarkPlugin{Dir: *pluginsDir}
	starlarkPlugin.SetParsers(activeParsers)
	starlarkPlugin.SetCorrelator(func(token string) []parsers.LogEntry {
		ringMu.Lock()
		defer ringMu.Unlock()
		var res []parsers.LogEntry
		if entries, ok := invertedIndex[token]; ok {
			for _, e := range entries {
				res = append(res, *e)
			}
		}
		return res
	})
	plugins.ActivePlugins = append(plugins.ActivePlugins, starlarkPlugin)

	for _, p := range plugins.ActivePlugins {
		if err := p.Init(); err != nil {
			safePrint("Warning: Failed to init plugin: %v\n", err)
		}
	}

	if *claude {
		plugins.ActiveDiagnostics = append(plugins.ActiveDiagnostics, &plugins.ClaudeDiagnostic{Enabled: true})
	}

	var esEngine *parsers.ElkEngine
	if esURL != "" {
		esEngine = parsers.NewElkEngine(esURL, esIndex, esUser, esPass)
	}

	safePrint("📋 afterloggrator (Modular Engine)\n")
	safePrint("Monitoring %d file(s):\n", len(files))
	for i, f := range files {
		color := ""
		if !*noColor {
			color = fileColors[i%len(fileColors)]
		}
		if i < 10 {
			safePrint("  %s[%d] %s%s\n", color, i+1, f, parsers.ColorReset)
		}
	}
	if len(files) > 10 {
		safePrint("  ... and %d more files\n", len(files)-10)
	}
	safePrint("%s\n\n", strings.Repeat("=", 80))

	logChan := make(chan parsers.LogEntry, 100000)
	workerChan := make(chan WorkerResult, 100000)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		cancel()
	}()

	var fileWg sync.WaitGroup
	var activeWg sync.WaitGroup
	var esWg sync.WaitGroup

	var allTargets []struct {
		Host string
		File string
	}

	if *sshFlag && *hostsFlag != "" {
		for _, h := range strings.Split(*hostsFlag, ",") {
			host := strings.TrimSpace(h)
			if host != "" {
				for _, f := range files {
					allTargets = append(allTargets, struct{ Host, File string }{host, f})
				}
			}
		}
	} else {
		for _, f := range files {
			allTargets = append(allTargets, struct{ Host, File string }{"", f})
		}
	}

	for i, target := range allTargets {
		fileWg.Add(1)
		color := ""
		if !*noColor {
			color = fileColors[i%len(fileColors)]
		}
		go func(h, f, c string) {
			defer fileWg.Done()
			if h != "" {
				extractors.StreamRemoteFile(ctx, h, *sshUser, *sshKey, f, *follow, c, logChan)
			} else {
				if *follow {
					tailFile(ctx, f, c, logChan)
				} else {
					catFile(f, c, logChan)
				}
			}
		}(target.Host, target.File, color)
	}

	numWorkers := 8
	for w := 0; w < numWorkers; w++ {
		activeWg.Add(1)
		go func() {
			defer activeWg.Done()
			for entry := range logChan {
				if entry.AppID == "" {
					for _, engine := range activeParsers {
						app, parsed := engine.Parse(entry.Line)
						if app != "" {
							entry.AppID = app
							entry.ParsedFields = parsed
							break
						}
					}
				}

				var lineTokens []string
				if *useRegex {
					lineTokens = findTokens(ipRegex, entry.Line)
					lineTokens = append(lineTokens, findTokens(macRegex, entry.Line)...)
				} else {
					lineTokens = parsers.FindTokensFast(entry.Line)
				}
				entry.Tokens = lineTokens

				plugins.Dispatch(&entry)

				isPrimaryMatch := false
				if *appFilter != "" && !strings.EqualFold(*appFilter, entry.AppID) {
				} else {
					if filterRegex != nil {
						match, _ := filterRegex.MatchString(entry.Line)
						if match {
							isPrimaryMatch = true
						}
					} else if *appFilter != "" {
						isPrimaryMatch = true
					} else {
						isPrimaryMatch = true
					}
				}

				workerChan <- WorkerResult{
					Entry:          entry,
					IsPrimaryMatch: isPrimaryMatch,
					LineTokens:     lineTokens,
				}
			}
		}()
	}

	go func() {
		fileWg.Wait()
		close(logChan) // Terminate workers after files are done
		esWg.Wait()
		activeWg.Wait()
		time.Sleep(1 * time.Second)
		close(workerChan)
	}()

	esLogChan := make(chan parsers.LogEntry, 1000)
	go func() {
		for entry := range esLogChan {
			func() {
				defer func() { recover() }() // Prevent channel close panic
				logChan <- entry
			}()
		}
	}()

	activeTokens := make(map[string]time.Time)
	queriedTokens := make(map[string]bool)
	tokenTTL := 5 * time.Minute
	
	// Perf 1: Inverted index
	// Declared and initialized earlier to be accessible inside Starlark closure!

	if *correlate {
		ringBuffer = make([]*parsers.LogEntry, 0, ringSize)
	}

	var collectedLogs []string

	for result := range workerChan {
		entry := result.Entry
		isPrimaryMatch := result.IsPrimaryMatch
		lineTokens := result.LineTokens

		isCorrelated := false

		if *correlate {
			ringMu.Lock()
			if isPrimaryMatch {
				for _, t := range lineTokens {
					if t != "127.0.0.1" && t != "0.0.0.0" {
						activeTokens[t] = time.Now().Add(tokenTTL)

						if esEngine != nil && !queriedTokens[t] {
							queriedTokens[t] = true
							esWg.Add(1)
							go func(token string) {
								defer esWg.Done()
								esEngine.Query(token, esLogChan)
							}(t)
						}
					}
				}
				for _, t := range lineTokens {
					if linkedEntries, ok := invertedIndex[t]; ok {
						for _, linked := range linkedEntries {
							if !linked.Printed {
								printEntry(*linked, len(files) > 1, *timestampFlag, *noColor, true)
								linked.Printed = true

								if *claude {
									collectedLogs = append(collectedLogs, fmt.Sprintf("[%s] %s", filepath.Base(linked.Filename), linked.Line))
								}
							}
						}
					}
				}
			} else {
				now := time.Now()
				for t, exp := range activeTokens {
					if now.After(exp) {
						delete(activeTokens, t)
						continue
					}
					for _, lineToken := range lineTokens {
						if lineToken == t {
							isCorrelated = true
							break
						}
					}
					if !isCorrelated && strings.Contains(entry.Line, t) {
						isCorrelated = true
					}
				}
			}

			if len(invertedIndex) > 50000 {
				invertedIndex = make(map[string][]*parsers.LogEntry)
			}

			if len(ringBuffer) >= ringSize {
				ringBuffer = ringBuffer[1:]
			}
			newEntry := entry
			ringBuffer = append(ringBuffer, &newEntry)
			
			if isPrimaryMatch || isCorrelated {
				for _, t := range lineTokens {
					invertedIndex[t] = append(invertedIndex[t], &newEntry)
				}
			}
			ringMu.Unlock()
		}

		if isPrimaryMatch || isCorrelated {
			if !entry.Printed {
				printEntry(entry, len(files) > 1, *timestampFlag, *noColor, isCorrelated && !isPrimaryMatch)

				if *correlate {
					ringMu.Lock()
					if len(ringBuffer) > 0 {
						ringBuffer[len(ringBuffer)-1].Printed = true
					}
					ringMu.Unlock()
				}

				if *claude {
					collectedLogs = append(collectedLogs, fmt.Sprintf("[%s] %s", filepath.Base(entry.Filename), entry.Line))
				}
			}
		}
	}

	if len(collectedLogs) > 0 {
		plugins.RunDiagnostics(collectedLogs)
	}
}
