# FAQ

## How long until it detects nesting pokemon?

At minimum, nothing will be updated in the DB until the configured 'min_history_duration_hours'. This defaults to 1h. At 1h, the nesting pokemon for large nests will likely be known.

After that, every 'rotation_interval_minutes', there's a chance at determining the nesting pokemon. This defaults to 15 minutes.

But it really depends on the number of spawns in each nest. The nesting pokemon will not be determined until there's a sufficient amount of stats.

You can `grep NESTING logs/fletchling.log` to easily see nesting pokemon decisions. You may see something like "299:1460". That's dexId:formId and that one happens to be Nosepass normal form.

## Why does the importer error with something about nests db migrations?

If there are DB schema changes in a new version of Fletchling, those migrations need to be run. The importer tool will not do these migrations because if old Fletchling is running, it is possible the migrations will break it. So, the importer always checks to make sure the DB is where it should be. If it is not, it means you've not restarting Fletchling yet. Restarting Fletchling will run the DB migrations. Then you may use the importer tool and it won't complain.

## I ran the importer, but I have no spawnpoint counts in my DB. What gives?

If nests were actually imported, there's only a couple of reasons why this can be:

* You forgot to configure the golbat_db section in the config file.
* The spawnpoint DB queries are erroring, possibly due to wrong golbat_db configuration.

After correcting the above, you can issue the `/api/config/reload?spawnpoints=all` API call to Fletchling to have it recompute everything and reload.
