# Fletchling

Fletchling is a Golbat webhook receiver that processes pokemon
webhooks and computes nesting pokemon.

# Features

* receives and processes pokemon on the fly via webhook from Golbat
* fletchling-importer (separate tool): Can copy nests from: overpass, db, or koji to db or koji.
* koji can be used as an authortative source for nests (optional)
* has an API to pull stats, purge stats, reload config, etc.
* highly configurable.

# Instructions

1. rename and edit configs in configs/
2. in golbat, add a new webhook. (i think restart is required.) it should look like this in the config:
```
[[webhooks]]
url = "http://fletchling-host:9042/webhook"
types = ["pokemon_iv"]
```
3. figure out how you want to run fletchling. there's a docker-compose example and a build.sh.
4. check the logs, db, whatever, and enjoy.

# Migrating from other nest processors:

(UNTESTED)

## nestcollector to fletching with database as authortative source (SIMPLEST)
  1. configure your existing golbat db in configs/fletchling.toml
  2. do not configure koji section.
  3. nuke your cronjob.
  4. `./fletchling`

## nestcollector to Fletchling with koji as authorative source.
  1. configure your existing golbat db in configs/fletchling-importer.toml in the 'db_exporter' section.
  2. configure your koji endpoint (BASE URL ONLY) in configs/fletchling-importer.toml in the 'koji_importer' section.
  3. create a project for nests in koji.
  4. `./build.sh`
  5. `./fletchling-importer -koji-dest-project 'YOURKOJIPROJECT' db koji`
  6. check koji and fix up whatever you want.
  7. configure a new db or existing golbat db in configs/fletchling.toml
  8. configure koji section in configs/fletchling.toml
  9. nuke your cronjob.
  10. `./fletchling`

## The old nestwatcher db is not supported currently.
