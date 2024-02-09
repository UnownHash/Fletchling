all: fletchling fletchling-osm-importer

deps:
	if [ ! -f vendor/modules.txt ]; then go mod download; fi

fletchling: deps
	CGO_ENABLED=0 go build -tags go_json ./bin/fletchling/...

fletchling-osm-importer: deps
	CGO_ENABLED=0 go build ./bin/fletchling-osm-importer/...

clean:
	rm -f fletchling fletchling-osm-importer
