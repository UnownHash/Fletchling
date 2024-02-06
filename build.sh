#!/bin/sh -e

export CGO_ENABLED=0

if [ ! -f vendor/modules.txt ]; then go mod download; fi
go build ./bin/fletchling/...
go build ./bin/fletchling-importer/...
