# Fletchling

Fletchling is a Golbat webhook receiver that processes pokemon
webhooks and computes nesting pokemon.

# Features

* Receives and processes pokemon on the fly via webhook from Golbat
* Uses global spawn data to not confuse event spawns (CD, Spotlight, etc) with nesting pokemon.
* No reliance on external sites for event or current nesting mon data.
* Tool for importing nests from overpass (fletchling-osm-importer)
* API to pull stats, purge stats, reload config, etc.

# Configuration

1. Copy configs/fletchling.toml.example to configs/fletchling.toml and edit it.
2. In golbat, add a new webhook entry to its config. It should look like this:

```
[[webhooks]]
url = "http://FLETCHLING-HOSTNAME:9042/webhook"
types = ["pokemon_iv"]
```

Replace 'FLETCHLING-HOSTNAME' with your host that is running Fletchling. If you are using pm2 and not using
docker, this should likely be '127.0.0.1'. If you are using docker, your fletchling hostname will likely be
'fletchling'.

After adding the config to Golbat, restart Golbat to have it take effect.

3. If you plan to use docker-compose, copy docker-compose.yml.example to docker-compose.yml and edit.

# Running

## docker

1. There is an included docker-compose.yml.example file. Copy it to docker-compose.yml and edit it, if needed.
2. `docker-compose up -d`

## pm2

### Requirements

You will need to have at least golang 1.22.1 installed. It is rather new as of the time of this writing. You may need to install it manually. See the [instructions and download links](https://go.dev/dl/).

### Building and starting

1. `make`
2. `pm2 start ./fletchling --name fletchling -o /dev/null`

# Verifying it is working

1. `curl http://localhost:9042/api/config` -- this should give you the current processor configuration
2. Check the logs in logs/ (or the log_dir that you configured in fletchling.toml) for errors.
   * Every minute, a log message will appear saying how many pokemon were processed. If this is 0, it means that Fletchling is not getting any webhooks. Check your Golbat webhooks configuration. Check the address Fletchling is listening on (http section in config).

# Prometheus / Grafana

Fletchling exposes Prometheus metrics (webhook processing counters, HTTP stats, Go runtime, and process metrics) on the same listen address as the API, at `/metrics`. The endpoint is **disabled by default** — you must opt in.

## 1. Enable the metrics endpoint in Fletchling

Edit `configs/fletchling.toml` and uncomment the `enabled` line:

```toml
[prometheus]
enabled = true
```

Restart Fletchling. Verify it works: `curl http://127.0.0.1:9042/metrics` should return a long list of `fletchling_*` metrics. If you get `404 page not found`, the endpoint isn't enabled — recheck your config.

## 2. Scrape it with Zapdos

[Zapdos](https://github.com/UnownHash/Zapdos) is the UnownHash metrics stack (VmAgent + VictoriaMetrics + Grafana). Follow its docs to get the stack running, then add a scrape job for Fletchling in your `prometheus.yml`:

```yml
scrape_configs:
  - job_name: 'fletchling'
    static_configs:
      - targets: ['fletchling:9042']   # docker; for pm2 use 127.0.0.1:9042
        labels:
          instance: 'fletchling'
```

Reload VmAgent so the new target is picked up.

## 3. Import the dashboard

A Grafana dashboard covering all exposed metrics is included at [`grafana/fletchling-dashboard.json`](grafana/fletchling-dashboard.json). In Grafana: **Dashboards → New → Import**, upload the file, and select your VictoriaMetrics/Prometheus datasource.

# Migrating from other nest processors

## nestcollector to Fletching using existing Golbat DB for nests (SIMPLEST)
  1. Gather your Golbat DB info.
  2. Create `configs/fletchling.toml` as per 'Configuration' instructions above.
     * configure your existing golbat db in configs/fletchling.toml in BOTH 'nests_db' and 'golbat_db' sections.
     * fix the listen address in 'http' section, if necessary.
  3. nuke your cronjob.
  4. start up fletchling per 'Running' section above.

## Others

I would just start over, personally. :)

# Importing OSM data

Importing docs can be found [here.](./docs/IMPORTING.md)

# API

API docs can be found [here.](./docs/API.md)

# FAQ

FAQ can be found [here.](./docs/FAQ.md)

# Credits

Thanks to the folks who put a lot of time and effort into nestcollector! Most of the importing, nest-activation filtering, and so forth were either copied/ported from nestcollector or used as a guide to achieve some amount of compatibility.

# Enjoy!

All your nest are belong to us.
