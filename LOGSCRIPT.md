# Starlark filters

Run `afterloggrator -script path/to/filter.star -json app.log`.
A script must export `filter(event)` and return `True` or `False`.

```python
load("common.star", "is_server_error")

def filter(event):
    return (
        event["cluster"] in ["prod-us", "prod-eu"]
        and event["service"] == "checkout"
        and is_server_error(event["fields"].get("status", ""))
        and regex_match("timeout|refused", event["line"])
    )
```

`common.star` in the same script directory:

```python
def is_server_error(status):
    return status in ["500", "502", "503", "504"]
```

## Event contract

| Key | Value |
| --- | --- |
| `source`, `host`, `service`, `container`, `cluster` | Source identity strings |
| `app_id`, `filename`, `line` | Recognized parser application, input path/name, original line |
| `timestamp` | UTC RFC3339 event time; empty string if unknown |
| `fields` | Flattened JSON/logfmt/parser fields as strings |
| `tokens` | Canonical IP, IPv6, MAC, and domain tokens |

Use `event["fields"].get("jsonPayload.request_id", "")` for nested variables.
Event data and module globals are frozen. Scripts select primary matches; they do
not modify logs or source identity. All CLI filters are ANDed with the script.
`-correlate-by` can add related context that does not pass the script, while still
respecting the global event-time bounds.

`regex_match(pattern, text)` uses Go's regular expression syntax (no lookbehind).
The CLI `-filter` uses regexp2 syntax. `print` writes to stderr, keeping JSON
stdout clean. Script runtime errors and nonboolean results fail the command.

`load` paths are relative to the entry script's directory, including imports from
nested modules. Modules must stay inside that directory; parent traversal,
escaping symlinks, and cyclic imports fail. Each module is limited to 1 MiB, each
load graph to 64 modules, and initialization and each filter call to 100,000
execution steps. Cancellation interrupts execution. Large allocations can still
consume memory; this is not an OS-level sandbox for hostile scripts.

Keep shared helpers in version control alongside filters. There are no external
I/O builtins and no automatic plugin discovery. The former `process_log(entry)`,
`exec`, `http_get`, `read_file`, DNS/RBL, and write-file interfaces are retired.
Perform enrichment before ingestion and include the resulting fields in logs.
