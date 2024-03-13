# API

## Get config
`curl http://localhost:9042/api/config`

## Reload configuration
`curl http://localhost:9042/api/config/reload`
(Also supports PUT. You can also send a SIGHUP signal to the process)

## Rerun spawnpoint, area, overlap filtering and reload configuration:
`curl http://localhost:9042/api/config/reload?refresh=1`

## Refresh spawnpoint counts, re-run spawnpoint, area, overlap filtering and reload configuration:
`curl http://localhost:9042/api/config/reload?refresh=1&spawnpoints=all`

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
`curl -X PUT http://localhost:9042/api/stats/purge/keep -d '{ "duration_minutes": xx }'`

This is another way to purge oldest stats. But with this one, you specify the duration to keep, not the duration to purge.

# Healthcheck status endpoint

## Get status
`curl http://localhost:9042/status`
