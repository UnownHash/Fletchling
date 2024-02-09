# Fletchling

Fletchling is a Golbat webhook receiver that processes pokemon
webhooks and computes nesting pokemon.

# Features

* receives and processes pokemon on the fly via webhook from Golbat
* fletchling-osm-importer (separate tool): Can copy nests from: overpass to db (or koji soon)
* Koji can be used as an authortative source for nests (soon)
* has an API to pull stats, purge stats, reload config, etc.

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

# Migrating from other nest processors

## nestcollector to Fletching with database as authortative source (SIMPLEST)
  1. Gather your Golbat DB info.
  2. Create `configs/fletchling.toml` from the existing example config.
     * configure your existing golbat db in configs/fletchling.toml in both 'nests_db' and 'golbat_db' sections.
     * fix the listen address in 'http' section, if necessary.
  4. nuke your cronjob.
  5. start up fletchling 

## nestcollector to Fletchling with Koji as authorative source

This will be available soon.

## Others

I would just start over, personally. :)

# Importing OSM data

Overpass API can be queried to find parks, etc, to import into your nests db (or Koji soon).

The fences/areas that are searched can come from either a poracle-style json file, or a geojson FeatureCollection file, or Koji(soon).

  1. If you want to find nests for your areas that are in Koji, make sure you have a project which exports your areas.
  2. If you want to find nests for your areas that are in a file, make note of its location.
  3. Gather your nests DB info/credentials. If you are currently using the Golbat DB, use this. Otherwise, if you are starting from scratch, use golbat or make a new database.
  4. (Optional): Gather your golbat DB info/credentials (might be same as Step 3.). min_spawnpoints filtering will only work with this configured.
  5. Create `configs/fletchling-osm-importer.toml` from the existing example config. The comments in the file should explain.
  6. `./make`
  7. `./fletchling-osm-importer 'AreaName'` to import a single area first, if you wish.
  8. `./fletchling-osm-importer -all-areas` to import all areas.

# API

## Get config
`curl http://localhost:9042/api/config`

## Reload configuration
`curl http://localhost:9042/api/config/reload`
(Also supports PUT. You can also send a SIGHUP signal to the process)

## Get all nests
`curl http://localhost:9042/api/nests`

## Get single nest
`curl http://localhost:9042/api/nests/:nest_id`

## Get all nests and full stats history
`curl http://localhost:9042/api/nests/_/stats`

## Get single nest and its stats history
`curl http://localhost:9042/api/nests/_/:nest_id`

Untested:

## Purge all stats
`curl -X PUT http://localhost:9042/api/stats/purge/all`

This ditches all stats history including the current time period. This starts the stats with a clean slate, but like startup.

## Purge some duration of oldest stats
`curl -X PUT http://localhost:9042/api/stats/purge/oldest -d '{ "duration_minutes": xx }'`

Purges the specified duration of the stats starting from the oldest. This will never remove the current unfinished time period. This can be used to nuke everything but the current time period by specifying a very high number of minutes.

## Purge some duration of newest stats
`curl -X PUT http://localhost:9042/api/stats/purge/newest -d '{ "duration_minutes": xx, "include_current": false }'`

Purges the specified amount of minutes of stats starting from the newest. 'include_current' specifies whether it should start with the current time period that is not done, or if it should start at the last period.

## Ensure only a certain duration of stats exists
`curl -X PUT http://localhost:9042/api/stats/purge/keep -d '{ "duration_minutes": xx }'

This is another way to purge oldest stats. But with this one, you specify the duration to keep, not the duration to purge.


# Known issues

The importer can import nests that are fully contained by other nests. For example, if a large park has a number of baseball fields, it is possible that nests for both the park and the fields will be imported. This will be fixed soon.

# Enjoy!

All your nest are belong to us.
