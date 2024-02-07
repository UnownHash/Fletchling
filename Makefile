all: fletchling fletchling-importer

deps:
	if [ ! -f vendor/modules.txt ]; then go mod download; fi

fletchling: deps
	CGO_ENABLED=0 go build -tags go_json ./bin/fletchling/...

fletchling-importer: deps
	CGO_ENABLED=0 go build ./bin/fletchling-importer/...

clean:
	rm -f fletchling fletchling-importer
