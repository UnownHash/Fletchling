package stats_collector

import "github.com/gin-gonic/gin"

var _ StatsCollector = (*noopCollector)(nil)

type noopCollector struct {
}

func (col *noopCollector) Name() string                   { return "no-op" }
func (col *noopCollector) RegisterGinEngine(*gin.Engine)  {}
func (col *noopCollector) AddPokemonProcessed(num uint64) {}
func (col *noopCollector) AddPokemonMatched(num uint64)   {}
func (col *noopCollector) AddNestsMatched(num uint64)     {}

func NewNoopStatsCollector() StatsCollector {
	return &noopCollector{}
}
