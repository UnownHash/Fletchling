# Importing OSM data

fletchling-osm-importer can be used to query the Overpass API to find parks, etc to import as nests into your db. The importer can be run multiple times without overwriting your existing nests.

The importer will use the same config file as fletchling itself. Make sure the 'areas' section is configured. When importing, you tell fletchling-osm-importer which areas to search. The fences/areas that are searched can come from either a poracle-style json file, or a geojson FeatureCollection file, or Koji.

## Configuration

  1. If you want to find nests for your areas that are in Koji, make sure you have a project which exports your areas.
  2. If you want to find nests for your areas that are in a file, make note of its name/location.
  3. Gather your Nests DB info/credentials. You may use the Golbat DB for this or a brand new DB.
  4. Gather your Golbat DB info/credentials. This will be used for spawnpoint filtering and may be the same as Step 3.
  5. Create `configs/fletchling.toml` as per 'Configuration' instructions for Fletchling above.
     * Make sure to configure the 'areas' section.
     * Optionally adjust the 'filter' and 'importer' sections to your liking.
  6. Ensure fletchling is running per the 'Running' section above. The importer will complain if DB migrations exist that have not been applied, and making sure Fletchling is running with the latest code ensures they've been applied.

## Running the importer under docker (docker-compose):

There is a simple wrapper script that will run the importer

  1. `./docker-osm-importer.sh 'AreaName'` to import a single area first, if you wish.
  2. `./docker-osm-importer.sh -all-areas` to import all areas.

## Running the importer if you don't use docker:

  1. `./fletchling-osm-importer 'AreaName'` to import a single area first, if you wish.
  2. `./fletchling-osm-importer -all-areas` to import all areas.

## Make your new nests or changes live.

The importer, by default, will gather spawnpoint counts for new nests (if golbat_db is configured) after it is done importing. It will also re-run filters based on all of your configuration under the 'filters' section in the config file. Nests that pass the filters will be activated and those that don't will be deactivated. This will apply to *all* nests in the Nests DB, not only the ones just added! This happens as the last step of the import and can take quite a while to figure out 'overlap percent'. This behavior can be disabled by adding the '-skip-activation' switch when running the importer. If you use this option, you will later need to issue an API to Fletchling to activate these imported nests. Even if you do not use it, you still need to tell Fletchling to reload:

### Reload fletchling

If you do not use -skip-activation when importing, all you need to do to make nests active is tell Fletchling to reload:

  * `curl http://FLETCHLING-HOSTNAME:9042/api/config/reload`

### Delaying Nest activation.

Since activation can take quite a while to run all of the filters (particularly the 'overlap percent' filter), it may be desireable to delay this activation until you know you are ready for it. For example, you may desire this type of flow:

  * `./fletchling-osm-importer -skip-activation Area1`
  * `./fletchling-osm-importer -skip-activation Area2`
  * `./fletchling-osm-importer -skip-activation Area3`
  * Have Fletchling do the filtering,activation,reload: `curl http://FLETCHLING-HOSTNAME:9042/api/config/reload?refresh=1`
