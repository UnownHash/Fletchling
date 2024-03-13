#!/bin/sh

# run fletchling-osm-importer under docker

cd `dirname $0`

which docker-compose > /dev/null
if [ $? -eq 0 ]; then
  command='docker-compose'
else
  command='docker compose'
fi

$command exec fletchling-tools ./fletchling-osm-importer "$@"
