ALL=fletchling fletchling-db-refresher fletchling-db-exporter fletchling-osm-importer

all: $(ALL)

deps:
	if [ ! -f vendor/modules.txt ]; then go mod download; fi

fletchling: deps
	CGO_ENABLED=0 go build -tags go_json ./bin/fletchling/...

fletchling-db-refresher: deps
	CGO_ENABLED=0 go build ./bin/fletchling-db-refresher/...

fletchling-db-exporter: deps
	CGO_ENABLED=0 go build ./bin/fletchling-db-exporter/...

fletchling-osm-importer: deps
	CGO_ENABLED=0 go build ./bin/fletchling-osm-importer/...

clean:
	rm -f $(ALL)
