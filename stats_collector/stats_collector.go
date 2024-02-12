package stats_collector

import (
	"github.com/gin-gonic/gin"
)

type StatsCollector interface {
	Name() string
	RegisterGinEngine(*gin.Engine)

	AddPokemonProcessed(num uint64)
	AddPokemonMatched(num uint64)
	AddNestsMatched(num uint64)
}

type Config interface {
	GetPrometheusConfig() PrometheusConfig
}

func GetStatsCollector(cfg Config) StatsCollector {
	promConfig := cfg.GetPrometheusConfig()
	if !promConfig.Enabled {
		return NewNoopStatsCollector()
	}
	return NewPrometheusCollector(promConfig)
}
