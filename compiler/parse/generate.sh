#!/bin/sh
set -eu

trap 'rm -f y.output' EXIT
go run golang.org/x/tools/cmd/goyacc@v0.34.0 -o parser.go parser.go.y
