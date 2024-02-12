package stats_collector

import (
	"github.com/Depado/ginprom"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

const (
	DEFAULT_PROMETHEUS_NAMESPACE = "fletchling"
)

type PrometheusConfig struct {
	Enabled    bool      `koanf:"enabled"`
	Token      string    `koanf:"token"`
	BucketSize []float64 `koanf:"bucket_size"`
	Namespace  string    `koanf:"namespace"`
}

func (cfg *PrometheusConfig) Validate() error {
	if !cfg.Enabled {
		return nil
	}
	// ...
	return nil
}

func GetDefaultPrometheusConfig() PrometheusConfig {
	return PrometheusConfig{
		BucketSize: []float64{.00005, .000075, .0001, .00025, .0005, .00075, .001, .0025, .005, .01, .05, .1, .25, .5, 1, 2.5, 5, 10},
		Namespace:  DEFAULT_PROMETHEUS_NAMESPACE,
	}
}

var _ StatsCollector = (*PrometheusCollector)(nil)

type PrometheusCollector struct {
	config   PrometheusConfig
	registry *prometheus.Registry

	pokemonProcessed prometheus.Counter
	pokemonMatched   prometheus.Counter
	nestsMatched     prometheus.Counter
}

func (col *PrometheusCollector) Name() string {
	return "prometheus"
}

func (col *PrometheusCollector) RegisterGinEngine(engine *gin.Engine) {
	p := ginprom.New(
		ginprom.Engine(engine),
		ginprom.Registry(col.registry),
		ginprom.Subsystem("gin"),
		ginprom.Path("/metrics"),
		ginprom.Token(col.config.Token),
		ginprom.BucketSize(col.config.BucketSize),
	)
	engine.Use(p.Instrument())
}

func (col *PrometheusCollector) AddPokemonProcessed(num uint64) {
	col.pokemonProcessed.Add(float64(num))
}

func (col *PrometheusCollector) AddPokemonMatched(num uint64) {
	col.pokemonMatched.Add(float64(num))
}

func (col *PrometheusCollector) AddNestsMatched(num uint64) {
	col.nestsMatched.Add(float64(num))
}

func NewPrometheusCollector(config PrometheusConfig) StatsCollector {
	ns := config.Namespace
	if ns == "" {
		ns = DEFAULT_PROMETHEUS_NAMESPACE
	}

	registry := prometheus.NewRegistry()
	collector := &PrometheusCollector{
		registry: registry,
		pokemonProcessed: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: ns,
				Name:      "pokemon_processed",
				Help:      "Total number of pokemon processed from webhook",
			},
		),
		pokemonMatched: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: ns,
				Name:      "pokemon_matched",
				Help:      "Total number of pokemon matching at least 1 nest",
			},
		),
		nestsMatched: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: ns,
				Name:      "nests_matched",
				Help:      "Total number of nests matched",
			},
		),
	}

	processOpts := collectors.ProcessCollectorOpts{
		Namespace: ns,
	}

	registry.MustRegister(
		collectors.NewProcessCollector(processOpts),
		collectors.NewGoCollector(
			collectors.WithGoCollectorRuntimeMetrics(
				collectors.MetricsGC,
				collectors.MetricsMemory,
			),
		),
		collector.pokemonProcessed,
		collector.pokemonMatched,
		collector.nestsMatched,
	)

	return collector
}
