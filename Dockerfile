# Build image
FROM golang:1.21-alpine as build

WORKDIR /go/src/app

COPY . .
RUN if [ ! -f vendor/modules.txt ]; then go mod download; fi
RUN CGO_ENABLED=0 go build -o /go/bin/fletchling ./bin/fletchling
RUN CGO_ENABLED=0 go build -o /go/bin/fletchling-osm-importer ./bin/fletchling-osm-importer
RUN mkdir /empty-dir

# Now copy it into our base image.
FROM gcr.io/distroless/static-debian11 as runner
COPY --from=build /go/bin/fletchling /go/bin/fletchling-osm-importer /fletchling/
COPY --from=build /empty-dir /fletchling/logs
COPY --from=build /go/src/app/db_store/sql /fletchling/db_store/sql

WORKDIR /fletchling
CMD ["./fletchling"]
