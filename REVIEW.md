# Code review and remediation

## Findings addressed

| Severity | Finding | Resolution |
| --- | --- | --- |
| Critical | SSH accepted any host key; remote filenames were interpolated as commands. | Required known_hosts verification, shell quoting, port/IPv6 handling, cancellation during handshake and reads. |
| High | Elasticsearch transport disabled TLS verification. | Removed insecure TLS; legacy client rejects redirects. New HTTPS sources verify certificates and reject redirects. |
| High | Starlark reused threads concurrently, ignored errors/results, and exposed arbitrary process/file/network access while described as isolated. | Explicit boolean filters, frozen globals/events, fresh threads, serialized calls, instruction limits, directory-confined shared imports, errors propagated. Old plugin interface fails initialization with migration guidance. |
| High | Source shutdown closed input while ES producers remained active; panic recovery concealed lost events. | Fixed source-worker ownership of channels; all producers finish before closure; no dynamic WaitGroup additions. Removed implicit ES feedback queries. |
| High | Correlation indexed only matching logs and retained evicted entries through index pointers. | Index all eligible arrivals; evict all references; emit direct matches in either arrival order without duplicate output. |
| High | Host identity was reduced to a basename, timestamps represented read time, and arbitrary JSON/logfmt variables were unavailable. | Preserve explicit source identity, event/observed timestamps, dotted fields, exact variable joins and aliases. |
| High | File/open/decode/scanner/source failures disappeared as successful empty output. | Errors propagate to a nonzero exit; sorted results withheld on failure; stream partial-output semantics documented. |
| High | Cloud support consisted of parsers, with no cloud query clients. | AWS and OCI official SDK clients; GCP and Azure HTTPS query clients; pagination and provider-specific request/response handling. |
| Medium | Tail dropped partial lines and missed ordinary rotation/truncation. | Bounded partial-line assembly, rename/recreate and observed truncate handling; poll-related limitations documented. |
| Medium | Archive members could be merged; ZIP errors were silently skipped; `.Z` used an incompatible raw LZW decoder. | Preserve member boundaries and errors; TGZ/TAR.GZ support; explicit `.Z` rejection. |
| Medium | Subprocess plugins created a goroutine per event, could interleave JSON, and had no lifecycle management. | Retired implicit executable discovery; initialization now explains migration to upstream enrichment. |
| Medium | AWS VPC parsing accepted arbitrary REJECT text; parser precedence depended on map order; custom regex matches were unbounded. | Structural VPC checks, deterministic syslog precedence, bounded regexp2 matches. |
| Medium | Tokenizer omitted domains/IPv6 and could overflow IPv4 octets. | Complete-token validation and canonicalization using standard address parsers. |
| Documentation | Claimed measured 100k+ EPS, zero-allocation parsing, unrestricted scripts as isolated, and all issues resolved. | Replaced claims with tested behavior, limits, examples, and breaking-change notes. |

## Verification

Passed: `go test -race ./...`, `go vet ./...`, and the documented multi-source
field-alias example. The example returns the earlier checkout context plus the
later database failure, ordered by event time across two hosts and clusters.

Regression tests cover cross-service/host/container/cluster field-alias correlation,
event-time ranges, retention, precision of raw JSON variables, source failure,
finite sorting limits, Starlark imports/escape attempts/cancellation/type errors/
step limits, archive boundaries, local rotation, HTTPS API-key/bearer authentication,
TLS/redirect rejection, Azure table/error handling, and empty-page pagination.
AWS and OCI tests make signed SDK requests to local TLS fixtures and verify
provider request shapes and pagination. They do not authenticate against live
provider accounts. See test files in `search`, `sources`, `parsers`, and `extractors`.

## Remaining limitations and explicit scope

- Live AWS/GCP/OCI/Azure, Docker, Kubernetes, and SSH deployments require operator
  validation; local protocol fixtures cannot prove environment-specific access.
- A DarkAPI logging contract has not been supplied or found in the public docs.
  The `darkapi` type is a configurable HTTPS adapter, not a verified fixed route.
- Cloud queries are finite, not persistent subscribers. No durable storage,
  distributed server, automatic cluster discovery, or cross-run cursor checkpoint.
- Correlation retains a bounded arrival window. Finite result sorting does not
  recover already evicted context. It links directly to primary matches only.
- Unknown/ambiguous event times stay unknown. RFC3164 year inference, arbitrary
  timestamp formats, multiline stack-trace assembly, and archive member identity
  are not implemented.
- Starlark instruction limits are not process memory limits. Shared scripts must
  be reviewed; hostile scripts can still allocate large values.
- GCP/Azure tokens are externally managed. Generic HTTPS paging is token-based
  JSON only; Link-header/offset/streaming protocols need a matching adapter.
- Generic HTTP clients report rate limits and partial errors, without automatic
  retries. AWS/OCI SDKs retain their retry policies.
- OCI SDK JSON result values use its numeric decoding behavior; large correlation
  IDs should be strings to avoid loss of precision above 2^53.
- Legacy Elasticsearch and diagnostic helper APIs are not used by the CLI. The ES
  helper still has a single-page query interface; it is not advertised as complete
  aggregation. Old prototype flags/configuration need migration (README).
- No throughput benchmark has been performed; there is no EPS or constant-time
  correlation guarantee. Memory bounds are primarily event counts plus per-line
  limits, not an absolute process-memory budget.

## Provider references consulted

- [AWS FilterLogEvents](https://docs.aws.amazon.com/AmazonCloudWatchLogs/latest/APIReference/API_FilterLogEvents.html)
- [GCP entries.list](https://docs.cloud.google.com/logging/docs/reference/v2/rest/v2/entries/list)
- [Azure query response format](https://learn.microsoft.com/en-us/azure/azure-monitor/logs/api/response-format)
- [OCI Logging Search](https://docs.oracle.com/en-us/iaas/Content/Logging/Concepts/using_the_api_searchlogs.htm)
- [OCI request signing](https://docs.oracle.com/en-us/iaas/Content/API/Concepts/signingrequests.htm)
- [DarkAPI public docs](https://darkapi.io/docs?tab=api)
