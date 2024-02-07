# Fletchling

Fletchling is a Golbat webhook receiver that processes pokemon
webhooks and computes nesting pokemon.

# Features

* receives and processes pokemon on the fly via webhook from Golbat
* fletchling-importer (separate tool): Can copy nests from: overpass, db, or Koji to db or Koji.
* Koji can be used as an authortative source for nests (optional)
* has an API to pull stats, purge stats, reload config, etc.
* highly configurable.

# Configuration

1. rename and edit configs in configs/
2. in golbat, add a new webhook. (i think restart is required.) it should look like this in the config:

```
[[webhooks]]
url = "http://your-fletchling-hostname:9042/webhook"
types = ["pokemon_iv"]
```

If you are using pm2, I would leave 'addr' in fletchling.toml as the default. Use '127.0.0.1' for your fletchling hostname in the above.

If you are using the included docker-compose.yml, golbat is also running under docker, and everything is attached to the same docker network, your fletchling hostname will be 'fletchling'.

# Running

## docker

1. There is an included docker-compose.yml.example file. Copy it to docker-compose.yml and edit it, if needed.
2. `docker-compose build`
3. `docker-compose up -d`

## pm2

1. `make`
2. `pm2 start ./fletchling --name fletchling -o /dev/null`

# Verifying it is working

1. curl http://localhost:9042/api/config -- this should give you the current processor configuration
2. The logfile is configurable in configs/fletchling.toml but defaults to logs/fletchling.log. Check it for errors.
3. Every minute, a log message will appear saying how many pokemon were processed. If this is 0, it means that Fletchling is not getting any webhooks. Check your Golbat webhooks configuration. Check the address Fletchling is listening on (http section in config).

# API

* Get config: `curl http://localhost:9042/api/config`
* Reload configuration: `curl http://localhost:9042/api/config/reload`
  (Also supports PUT. You can also send a SIGHUP signal to the process)
* Get all nests: `curl http://localhost:9042/api/nests`
* Get single nest: `curl http://localhost:9042/api/nests/:nest_id`
* Get all nests and full stats history: `curl http://localhost:9042/api/nests/_/stats`
* Get single nest and its stats history: `curl http://localhost:9042/api/nests/_/:nest_id`

Untested:

* Purge all stats: `curl -X PUT http://localhost:9042/api/stats/purge/all`

This ditches all stats history including the current time period. This starts the stats with a clean slate, but like startup.

* Purge some duration of oldest stats: `curl -X PUT http://localhost:9042/api/stats/purge/oldest -d '{ "duration_minutes": xx }'`

Purges the specified duration of the stats starting from the oldest. This will never remove the current unfinished time period. This can be used to nuke everything but the current time period by specifying a very high number of minutes.

* Purge some duration of newest stats: `curl -X PUT http://localhost:9042/api/stats/purge/newest -d '{ "duration_minutes": xx, "include_current": false }'`

Purges the specified amount of minutes of stats starting from the newest. 'include_current' specifies whether it should start with the current time period that is not done, or if it should start at the last period.

* Ensure only a certain duration of stats exists: `curl -X PUT http://localhost:9042/api/stats/purge/keep -d '{ "duration_minutes": xx }'

This is another way to purge oldest stats. But with this one, you specify the duration to keep, not the duration to purge.

# Migrating from other nest processors:

(UNTESTED)

## nestcollector to Fletching with database as authortative source (SIMPLEST)
  1. configure your existing golbat db in configs/fletchling.toml
  2. do not configure Koji section.
  3. nuke your cronjob.
  4. start up fletchling 

## nestcollector to Fletchling with Koji as authorative source.
  1. configure your existing golbat db in configs/fletchling-importer.toml in the 'db_exporter' section.
  2. configure your Koji endpoint (BASE URL ONLY) in configs/fletchling-importer.toml in the 'koji_importer' section.
  3. create a project for nests in Koji.
  4. `make`
  5. `./fletchling-importer -koji-dest-project 'NESTS-PROJECT' db koji` - If you want properties to be created in Koji if they are missing, also pass -
  6. check Koji and fix up whatever you want.
  7. configure a new db or existing golbat db in configs/fletchling.toml
  8. configure Koji section in configs/fletchling.toml
  9. nuke your cronjob.
  10. start up fletchling 

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
