# Build image
FROM golang:1.22-alpine as build

WORKDIR /go/src/app

COPY . .
RUN find db_store/sql -name '*~' -delete
RUN if [ ! -f vendor/modules.txt ]; then go mod download; fi
RUN CGO_ENABLED=0 go build -tags go_json -o /go/bin/fletchling ./bin/fletchling
RUN CGO_ENABLED=0 go build -o /go/bin/fletchling-osm-importer ./bin/fletchling-osm-importer
RUN CGO_ENABLED=0 go build -o /go/bin/fletchling-db-refresher ./bin/fletchling-db-refresher
RUN CGO_ENABLED=0 go build -o /go/bin/fletchling-db-exporter ./bin/fletchling-db-exporter
RUN CGO_ENABLED=0 go build -o /go/bin/sleep ./bin/sleep
RUN mkdir /empty-dir

# Now copy it into our base image.
FROM gcr.io/distroless/static-debian11 as runner
COPY --from=build /empty-dir /fletchling/logs
COPY --from=build /go/src/app/db_store/sql /fletchling/db_store/sql
COPY --from=build /go/bin/fletchling /go/bin/fletchling-osm-importer /go/bin/fletchling-db-refresher /go/bin/fletchling-db-exporter /go/bin/sleep /fletchling/

WORKDIR /fletchling
CMD ["./fletchling"]
