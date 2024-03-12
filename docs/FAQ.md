# FAQ

## How long until it detects nesting pokemon?

At minimum, nothing will be updated in the DB until the configured 'min_history_duration_hours'. This defaults to 1h. At 1h, the nesting pokemon for large nests will likely be known.

After that, every 'rotation_interval_minutes', there's a chance at determining the nesting pokemon. This defaults to 15 minutes.

But it really depends on the number of spawns in each nest. The nesting pokemon will not be determined until there's a sufficient amount of stats.

You can `grep NESTING logs/fletchling.log` to easily see nesting pokemon decisions. You may see something like "299:1460". That's dexId:formId and that one happens to be Nosepass normal form.
