# LogScript Specification

Afterloggrator integrates **Starlark**, allowing security analysts and infrastructure engineers to write isolated, dynamic event processors.

LogScripts (referred to internally as *Scriptlets*) do not require recompilation and are executed natively and securely within the Go logging pipeline.

## Defining a Scriptlet
All scriptlets must end with the `.star` extension and reside in your configured plugins directory (e.g. `scriptlets/`).
Every plugin **must** define a globally accessible function named `process_log(entry)`:

```python
def process_log(entry):
    # Your arbitrary alerting, formatting, or routing logic resides here
    pass
```

### The `entry` Dictionary Object
The `entry` parameter is populated natively by the Afterloggrator worker pool for every line.

| Key | Type | Description |
| :--- | :--- | :--- |
| `"line"` | String | The raw, unformatted log string from the ingest file. |
| `"app_id"` | String | The auto-detected application (e.g., `"sshd"`, `"nginx"`). |
| `"tokens"` | List of Strings | Pre-extracted IPv4 and MAC address tokens dynamically resolved by the hardware tokenizer. |

---

## LogScript Standard Library (Extremely Powerful)

LogScript isn't just a basic interpreter—it provides severe integration capabilities through an extensive **Native Go Standard Library**. You can perform threat intel lookups, execute external shell commands, and interact with REST APIs instantly out of the box.

### Networking & Threat Intel
- `resolve_ip(ip: string) -> string`: Performs a PTR reverse DNS lookup on an IP address, returning the hostname. 
- `resolve_name(name: string) -> string`: Performs a forward DNS lookup on a hostname, returning the resolved IP.
- `check_rbl(ip: string, rbl_domain: string) -> bool`: Checks an IP address against a designated Real-time Blackhole List (e.g. `zen.spamhaus.org`). Natively handles the backward DNS structuring needed for EBL integrations.

### Deep Correlation & Parsing
- `parse_log(line: string) -> dict`: Manually invokes the internal Afterloggrator mapping engine against an arbitrary string. Returns a dictionary mapping identical to the baseline `entry` parameter.
- `correlate(token: string) -> list[dict]`: Taps securely into the cross-goroutine Mutex-locked Ring Buffer Index to find all recent log lines connected to a specific token (`IP` or `MAC`), returning a list of `entry` maps.

### Subprocess Execution & Regex
- `exec(command: string, args: list[string]) -> string`: Native bridging into the OS Subprocess driver. Executes an arbitrary command (e.g., triggering an AWS Lambda function via CLI or blocking an IP instantly in `iptables`) and returns the `STDOUT` payload.
- `regex_match(pattern: string, text: string) -> bool`: Invokes the standard PCRE regex engine to deeply validate custom string rules if basic tokenization falls short.

### HTTP Webhooks & REST Endpoints
- `http_get(url: string) -> string`: Performs a GET request to an external server. Perfect for pinging internal analytical APIs (timeout locked at 5s to prevent stalled pipelines).
- `http_post(url: string, body: string, content_type: string) -> string`: Fires a POST request. Commonly used to funnel specific high-end threats immediately into Slack, PagerDuty, or Discord webhooks.

### File I/O
- `read_file(path: string) -> string`: Ingests an entire file asynchronously. Returns an empty string if permission is denied.
- `read_lines(path: string) -> list[string]`: Reads a file and natively yields it as a Starlark-iterable array of lines.
- `write_file(path: string, data: string) -> bool`: Atomically overwrites a file on disk with string data. Returns `True` on success.
- `append_file(path: string, data: string) -> bool`: Opens a file and securely appends data to the end. Excellent for building out isolated CSV/JSON ledgers dynamically based on filtered alerts within Starlark logic.

## Execution Guardrails
- **Concurrency**: `process_log` is called rapidly from asynchronous log orchestration workers executing synchronously. 
- **Time Complexity**: While `http_get` and `exec` are phenomenally powerful, blocking inside `process_log` will constrain the pipeline's overall throughput limit! Use high-latency integrations strictly on filtered edge-case alerts, rather than unconditionally invoking REST calls natively across every single log line!
