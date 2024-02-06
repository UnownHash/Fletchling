# Fletchling

Fletchling is a Golbat webhook receiver that processes pokemon
webhooks and computes nesting pokemon.

# Features

* receives and processes pokemon on the fly via webhook from Golbat
* fletchling-importer (separate tool): Can copy nests from: overpass, db, or Koji to db or Koji.
* Koji can be used as an authortative source for nests (optional)
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

## nestcollector to Fletching with database as authortative source (SIMPLEST)
  1. configure your existing golbat db in configs/fletchling.toml
  2. do not configure Koji section.
  3. nuke your cronjob.
  4. `./fletchling`

## nestcollector to Fletchling with Koji as authorative source.
  1. configure your existing golbat db in configs/fletchling-importer.toml in the 'db_exporter' section.
  2. configure your Koji endpoint (BASE URL ONLY) in configs/fletchling-importer.toml in the 'koji_importer' section.
  3. create a project for nests in Koji.
  4. `./build.sh`
  5. `./fletchling-importer -koji-dest-project 'NESTS-PROJECT' db koji` - If you want properties to be created in Koji if they are missing, also pass -
  6. check Koji and fix up whatever you want.
  7. configure a new db or existing golbat db in configs/fletchling.toml
  8. configure Koji section in configs/fletchling.toml
  9. nuke your cronjob.
  10. `./fletchling`

## The old nestwatcher db is not supported currently.

# Importing OSM data

Overpass API can be queried to find parks, etc, to import into either Koji or your nests db.

The fences/areas that are searched can come from either a poracle-style geofences.json or from Koji.

The example Koji project names below are only examples, of course. Choose your own.

## Koji for areas and Koji for nests
  1. Make sure you have a Koji project ('AREAS-PROJECT') that exports all of your geofences for your areas.
  2. configure your Koji base url in the 'koji_overpass_source' section of configs/fletching-importer.toml
  3. configure your nests db in the 'db_exporter' section in configs/fletchling.toml
  4. `./fletchling-importer -overpass-areas-src koji -overpass-koji-project 'AREAS-PROJECT' -overpass-area 'AREA-NAME' overpass db`
  5. Repeat this for every area you have, subtituting the 'AREA-NAME' you want to import nests for.

## Koji for areas and DB for nests
  1. Make sure you have a Koji project ('AREAS-PROJECT') that exports all of your geofences for your areas.
  2. configure your Koji base url in the 'koji_overpass_source' section of configs/fletching-importer.toml
  3. Make sure you have a Koji project created for importing the results ('NESTS-PROJECT'). These need to be separate project names or Koji instances.
  4. configure your target Koji base url in the 'koji_importer' section of configs/fletching-importer.toml
  5. `./fletchling-importer -koji-dest-project 'NESTS-PROJECT' -overpass-areas-src koji -overpass-koji-project 'AREAS-PROJECT' -overpass-area 'AREA-NAME' overpass Koji`
  6. Repeat this for every area you have, subtituting the 'AREA-NAME' you want to import nests for.

## A geofences.json file for areas

Most of the options are the same as the above examples, but skip the Koji AREAS-PROJECT part. Drop the 'overpass-koji-project' argument. Change '-overpass-areas-src' from 'Koji' to a filename containing your areas.

You'll end up with something like this:

### Importing into DB

 `./fletchling-importer -overpass-areas-src some-geofences-file.json -overpass-area 'AREA-NAME' overpass db`

### Importing into Koji

`./fletchling-importer -overpass-areas-src some-geofences-file.json -koji-dest-project 'NESTS-PROJECT' -overpass-area 'AREA-NAME' overpass Koji`

# Enjoy!

All your nest are belong to us.
