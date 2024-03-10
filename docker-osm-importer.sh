#!/bin/sh

# run fletchling-osm-importer under docker

cd `dirname $0`

docker-compose exec fletchling-tools ./fletchling-osm-importer "$@"
