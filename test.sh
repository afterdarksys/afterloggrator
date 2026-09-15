#!/bin/sh
set -eu
go test ./...
go run . -sources examples/sources.json -contains failure -correlate-by request_id=trace_id -sort -json
