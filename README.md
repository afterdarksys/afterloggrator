# Afterloggrator

A high-performance modular Go utility designed to tail, isolate, and aggregate massive volumes of logs (+100k EPS). It features automatic application profiling, IP/MAC correlation, structured enterprise parsing (AWS, Azure, GCP, Palo Alto, Docker), and native [Starlark scripting extensions](LOGSCRIPT.md).

## Features

- **Extreme Throughput Pipeline**: Uses an asynchronous worker pool and a zero-allocation tokenizer for processing multi-gigabyte files.
- **Auto-Correlation**: Automatically maps contextual events across split files (e.g. tracking an IP from `iptables.log` blocking into `auth.log` success) using an O(1) Inverted Index.
- **Embedded LogScript**: Write dynamic plugins using native Starlark scripts without recompiling.
- **Archive Extraction**: Natively stream-tails compressed `.gz`, `.zip`, and `.tar` logs directly, saving RAM.
- **AI Diagnostics**: Integrates Claude directly into the pipeline (`--claude`) to heuristically assess aggregated threats via the Anthropic API.
- **Configuration Parsing**: Supports setting global defaults via `/etc/afterlogger/config.cfg` and `~/.afterlogger/config.cfg`.
- **Syntax Highlighting**: Real-time coloring natively mapped to log levels (`ERROR`, `WARN`, `INFO`).

## Installation

```bash
go build -o afterloggrator .
```

## Basic Usage

### Tail a single file
```bash
./afterloggrator -f /var/log/app.log
```

### Monitor multiple files
```bash
./afterloggrator -f /var/log/app1.log /var/log/app2.log /var/log/app3.log
```

### Distributed SSH Clustering
Continuously multiplex and correlate logs off remote target endpoints exclusively over secure execution buffers natively.
```bash
./afterloggrator -f --ssh --hosts="192.168.1.100, 192.168.1.101" -correlate /var/log/auth.log
```
*(Customize connections via `--ssh-user` and `--ssh-key` flag overrides).*

### Filter via Hardware Tokenizer (Lightning Fast)
```bash
./afterloggrator -f -correlate /var/log/*.log
```

### Filter via Full PCRE Regex
```bash
./afterloggrator -f -correlate --use-regex -filter "(?i)admin|root" /var/log/auth.log
```

## Options

- `-f`: Follow log files (like `tail -f`)
- `-ssh`: Use SSH to pull logs natively from remote clusters
- `-hosts`: Comma-separated list of remote hostnames or IPs
- `-ssh-user`: Set remote SSH user (defaults to local `$USER`)
- `-ssh-key`: File path override for RSA/Ed25519 identity key
- `-filter`: Filter lines matching pattern
- `-use-regex`: Use PCRE `regexp2` engine for IP/MAC extraction instead of the high-performance Tokenizer.
- `-i`: Case-insensitive filtering
- `-t`: Add timestamp to each line
- `-no-color`: Disable colored output
- `-d`: Scan directories recursively for log files
- `-app`: Filter by parsed application origin (e.g., `sshd`, `iptables`, `nginx`)
- `-correlate`: Automatically extract IPs/domains/MACs to link events between different log files.
- `-sys`: Automatically include host system logs (`/var/log/syslog`, etc)
- `-claude`: Use Claude to automatically diagnose matched logs (requires `ANTHROPIC_API_KEY`)
- `-es-url`, `-es-index`, `-es-user`, `-es-pass`: Hook into an Elasticsearch cluster.

## Configuration Files

If running repeatedly, you can persist flags in an INI-style config format implicitly parsed at startup:
- `/etc/afterlogger/config.cfg`
- `~/.afterlogger/config.cfg`

**Example `config.cfg`:**
```ini
correlate=true
no-color=false
sys=true
use-regex=false
plugins-dir=/opt/afterloggrator/scriptlets
```
*Note: CLI args directly override any matching configuration file parameters.*

## Advanced LogScripting 

Explore the [LogScript Specification](LOGSCRIPT.md) to learn how to write isolated Python-like `.star` event processors to triage logs on the fly!
