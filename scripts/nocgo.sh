#!/bin/sh
# Fail if any non-standard package in the build graph, test dependencies
# included, uses C interop: cgo, C or C++ sources, or prebuilt .syso objects.
# Pure-Go assembly is allowed. CGO_ENABLED=1 so cgo files are listed at all.
set -eu
out=$(CGO_ENABLED=1 go list -deps -test \
	-f '{{if and (not .Standard) (or .CgoFiles .CFiles .CXXFiles .SysoFiles)}}{{.ImportPath}}{{end}}' \
	./...)
if [ -n "$out" ]; then
	echo "C interop is not allowed. These packages use cgo, C sources or prebuilt objects:" >&2
	echo "$out" >&2
	exit 1
fi
