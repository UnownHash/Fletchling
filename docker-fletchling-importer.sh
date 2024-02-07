#!/bin/sh

# run fletchling-importer under docker

cd `dirname $0`

docker-compose --profile cli run --rm fletchling-importer "$@"
