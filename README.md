# Afterloggrator

A Go command-line tool for searching and collating logs across files, hosts,
containers, clusters, and cloud logging services. Sources run concurrently and
feed one bounded processing pipeline. There is no measured throughput guarantee.

## Build and test

Requires Go 1.25 or later.

```sh
go build -o afterloggrator .
go test -race ./...
go vet ./...
```

## Search across sources

```sh
./afterloggrator -sources examples/sources.json \
  -contains 'failure' -correlate-by 'request_id=trace_id' \
  -since 2026-09-01T00:00:00Z -until 2026-09-02T00:00:00Z \
  -window 10m -sort -json
```

Edit the example to reference your actual files. Each source has a unique name
and optional `host`, `service`, `container`, `cluster`, and `labels`. Define
multiple sources to read different services on different hosts and clusters.
Full source identity is preserved in JSON output, including for identical filenames.

```sh
# Literal conditions are ANDed; field conditions use exact string equality.
./afterloggrator -contains timeout -contains payment -where cluster=prod \
  -where status=500 -json app.log worker.log

# regexp2 pattern syntax, with a 100 ms match timeout.
./afterloggrator -filter '(?i)error|fatal' -correlate -window 5m app.log

# Time-ordered results and a shared Starlark filter.
./afterloggrator -sources examples/sources.json -script examples/operations.star -sort -json

# Local and SSH follow start at the end of current files.
./afterloggrator -f -t /var/log/app.log
./afterloggrator -f -ssh -hosts host-a,host-b -ssh-user ops \
  -ssh-key ~/.ssh/id_ed25519 /var/log/app.log
```

All flags must precede positional file paths (Go's flag parser stops at the first
positional argument). `-d` scans local directories recursively. `-sys` adds
platform system log paths. Unreadable sources are errors.

### Matching and correlation

- `-filter`: regexp2 pattern; `-i` makes this pattern case insensitive.
- Repeat `-contains` for required literal strings; repeat `-where field=value`
  for required variables or source metadata. Nested JSON fields use dotted paths,
  such as `jsonPayload.request_id`. Raw JSON values retain numeric precision as
  strings; OCI SDK numeric results have the precision limitation noted in REVIEW.md.
- `-app` selects a recognized parser application. `service` is separately retained.
- `-since` / `-until`: inclusive RFC3339 event-time bounds. Cloud services may have
  their own boundary semantics (AWS's server-side end time is exclusive).
- `-correlate`: direct IP, IPv6, MAC, and domain matches. Domain recognition is
  syntactic; dotted filenames may also look like domains.
- `-correlate-by request_id,user_id`: joins equal values of either field.
  `-correlate-by request_id=trace_id` joins aliases with different variable names.
- `-window`: maximum absolute event-time difference from a primary match (default
  five minutes). `-window 0` removes this restriction, including for undated logs.
- Correlation includes earlier nonmatches and later related events, across all
  configured sources. Matches are direct links to primary results, not transitive
  graph traversal. Context can fail the primary string, field, or script filter;
  it must still satisfy the global time bounds.
- `-buffer` retains the most recent 5,000 arrivals by default. Eviction removes
  index references. Older context is unavailable; increase the buffer for larger
  investigations. This is an in-memory tool, not a durable log database.
- `-sort` orders selected finite results by event time, then source and line.
  Unknown times sort last. `-max-results` bounds the sort buffer; exceeding it
  returns an error without printing a truncated sorted result.

`timestamp` is event time; `observed_at` is collection time. RFC3339 JSON fields,
common Unix epoch fields, timestamp-prefixed container logs, and RFC5424 syslog
are recognized. Undated logs and RFC3164 timestamps without a year retain zero
event time. They are excluded when explicit time bounds are requested. `-t`
prints `unknown` for those events. JSON uses Go's zero timestamp for unknown time.

## Source types and authentication

See [examples/cloud-sources.json](examples/cloud-sources.json). Copy only the
sources you use and replace all placeholders.

| Type | Data source | Authentication / setup |
| --- | --- | --- |
| `file` | Plain logs, gzip, bzip2, ZIP, TAR, TAR.GZ, TGZ | Local permissions |
| `ssh` | Remote `cat` or `tail -n 0 -F` | Private key, user, required known_hosts verification |
| `docker` | `docker logs --timestamps` | Installed Docker CLI; optional named `context` |
| `kubernetes` | `kubectl logs` for a pod/container | Installed kubectl, explicit context, namespace, pod, container |
| `aws` | CloudWatch Logs FilterLogEvents | Official AWS SDK credential chain; optional profile; region and log group in `resource` |
| `gcp` | Cloud Logging entries.list | OAuth bearer token from `token_env`; `resource` is `projects/ID` |
| `azure` | Azure Monitor Log Analytics query | Entra bearer token from `token_env`; workspace ID in `resource`; KQL in `query` |
| `oci` | OCI Logging SearchLogs | Official OCI SDK configuration with tenancy/user OCIDs, fingerprint, signing private key; optional `oci_config` and profile |
| `https`, `darkapi` | Configurable HTTPS log search endpoint | Bearer token, API key, or custom headers sourced from environment variables |

AWS `query` is a CloudWatch filter pattern. GCP `query` is a Logging filter.
Azure `query` is KQL. OCI `query` is an OCI Logging search expression and requires
both `-since` and `-until`. Use queries returning individual logs, retaining event
timestamps; aggregate query rows may lack the fields needed for local filtering.

An **OCID identifies an OCI resource; it is not an authentication secret**. OCI
requests are signed using the SDK and the configured private key. GCP and Azure
tokens must be obtained through your existing identity workflow; this tool does
not refresh those tokens. AWS/OCI support the credential mechanisms provided by
their configured SDK providers. Encrypted SSH keys and SSH agents are not yet supported.

Cloud/HTTPS sources perform finite searches; `-f` rejects them. AWS, GCP and OCI
pagination is consumed to completion, including empty intermediate pages. Azure
partial-result errors are surfaced; divide oversized queries into smaller time
ranges. GCP/Azure/generic HTTP failures, including rate limits, return errors;
only AWS/OCI use SDK retries. Cloud permissions and live account connectivity
must be validated in your environment.

`-concurrency` defaults to eight sources. Follow mode requires it to be at least
the number of sources so every long-running source starts. Local follow preserves
partial lines and detects rename/recreate rotation and observed truncation. A file
truncated and regrown between polls can evade detection. Archives are finite reads
only; member boundaries are separated but member filenames are not exposed.
Unix compress `.Z` is explicitly unsupported.

### DarkAPI and custom HTTPS

A `darkapi` source uses the same configurable transport as `https`. No DarkAPI
logging route or response schema is assumed: the public documentation inspected
did not establish a logging search contract. Supply your deployment's endpoint
and mapping; live DarkAPI compatibility remains unverified.

```json
{
  "sources": [{
    "name": "darkapi-production",
    "type": "darkapi",
    "url": "https://YOUR-LOG-ENDPOINT/search",
    "method": "POST",
    "body": {"query": "service:checkout"},
    "api_key_env": "DARKAPI_API_KEY",
    "api_key_header": "X-API-Key",
    "records_path": "data.logs",
    "next_token_path": "next",
    "page_param": "cursor"
  }]
}
```

The example schema is illustrative, not a claimed DarkAPI API contract. Supported
responses: JSON array of log objects/strings; an object with a configured dotted
`records_path`; a single log object when no path is set; or `format: "ndjson"` /
`"text"`. JSON pagination uses a string token at `next_token_path`, sent as
`page_param` in the GET query or POST body. Omit both if the endpoint is unpaginated.
Server-side query/time parameters for generic HTTPS belong in `url` or `body`;
CLI filters are always applied locally after retrieval.

Use `token_env` for a bearer token, `api_key_env` for an API key, or
`headers_env: {"Header-Name": "ENV_VAR"}` for deployment-specific authentication.
Missing environment variables fail validation. URLs must use HTTPS without URL
credentials. TLS certificates are verified; redirects are refused. Each HTTP
response is limited to 32 MiB and each scanned line to 10 MiB.

## Starlark

See [LOGSCRIPT.md](LOGSCRIPT.md) for maintainable, shareable filters and imports.
Scripts execute with frozen event data and an instruction budget, without exposed
network, filesystem, or process builtins. Errors stop the search. This is not an
OS-level memory sandbox; review shared scripts before running them.

## Errors, migration, and limits

Failures return a nonzero exit status. Streaming output already written before a
source failure can be partial; sorted output is withheld if any source fails.
There are no silent skips or panic-recovery channel shutdowns in the new pipeline.

The prior README/ISSUES asserted unmeasured 100k+ EPS performance, isolation of
scripts that could execute commands, and fully resolved race conditions. Those
claims were incorrect. See [REVIEW.md](REVIEW.md) for findings and remaining work.

The CLI now uses explicit `-sources` JSON configuration. Implicit INI defaults,
`-plugins-dir`, executable subprocess discovery, `process_log`, `-claude`, legacy
Elasticsearch flags, `-rules`, and `-use-regex` are retired. They are rejected as
unknown flags rather than silently changing search behavior. `-no-color` remains
accepted; output is always plain text. Legacy plugin/Elasticsearch Go helpers
remain for source compatibility but are not used by the CLI. Use the configurable
HTTPS adapter for finite custom endpoint queries; it is not a native Elasticsearch
scroll client. These are breaking changes from the prototype interface.
