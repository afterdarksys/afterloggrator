#!/bin/sh
set -eu
go run . -sources examples/sources.json -script examples/operations.star -sort -json
