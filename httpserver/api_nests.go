package httpserver

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/paulmach/orb/geojson"

	"github.com/UnownHash/Fletchling/processor"
	"github.com/UnownHash/Fletchling/processor/models"
)

type APINest struct {
	Id             int64                      `json:"id"`
	Name           string                     `json:"name"`
	Lat            float64                    `json:"lat"`
	Lon            float64                    `json:"lon"`
	Geometry       *geojson.Geometry          `json:"geometry,omitempty"`
	AreaName       *string                    `json:"area_name"`
	Spawnpoints    *int64                     `json:"spawnpoints"`
	AreaM2         float64                    `json:"area_m2"`
	Active         bool                       `json:"active,omitempty"`
	Discarded      *string                    `json:"inactive_reason,omitempty"`
	UpdatedAt      time.Time                  `json:"updated_at"`
	NestingPokemon *models.NestingPokemonInfo `json:"nesting_pokemon"`
}

type getNestsResponse struct {
	Nests []*APINest `json:"nests"`
}

type getOneNestResponse struct {
	Nest *APINest `json:"nest"`
}

func nestToAPINest(nest *models.Nest, includeGeometry bool) *APINest {
	var discarded *string

	if !nest.Active {
		discarded = &nest.Discarded
	}

	ni, updatedAt := nest.GetNestingPokemon()

	center := nest.Center
	apiNest := &APINest{
		Id:             nest.Id,
		Name:           nest.Name,
		Lat:            center.Lat(),
		Lon:            center.Lon(),
		AreaName:       nest.AreaName,
		Spawnpoints:    nest.Spawnpoints,
		AreaM2:         nest.AreaM2,
		Active:         nest.Active,
		Discarded:      discarded,
		UpdatedAt:      updatedAt,
		NestingPokemon: ni,
	}

	if includeGeometry {
		apiNest.Geometry = nest.Geometry
	}

	return apiNest
}

// Currently only returns active nests.
func (srv *HTTPServer) handleGetNests(c *gin.Context) {
	nests := srv.nestProcessorManager.GetNests()
	apiNests := make([]*APINest, len(nests))

	for idx, nest := range nests {
		apiNests[idx] = nestToAPINest(nest, false)
	}

	c.JSON(http.StatusOK, getNestsResponse{apiNests})
}

// Currently only returns active nests.
func (srv *HTTPServer) handleGetNest(c *gin.Context) {
	nestId, err := strconv.ParseInt(c.Param("nest_id"), 10, 64)
	if err != nil {
		srv.logger.Warnf("GetNest: bad nest id: %v", err)
		c.JSON(http.StatusBadRequest, &APIErrorResponse{
			Error: "malformed nest ID",
		})
		return
	}

	nest := srv.nestProcessorManager.GetNestByID(nestId)
	if nest == nil {
		c.JSON(http.StatusNotFound, &APIErrorResponse{
			Error: "Nest not found",
		})
		return
	}

	apiNest := nestToAPINest(nest, true)

	c.JSON(http.StatusOK, getOneNestResponse{apiNest})
}

// Currently only returns active nests.
func (srv *HTTPServer) handleGetNestStats(c *gin.Context) {
	nestProcessor := srv.nestProcessorManager.GetNestProcessor()

	var nests []*models.Nest

	nestIdStr := c.Param("nest_id")
	if nestIdStr != "" {
		nestId, err := strconv.ParseInt(c.Param("nest_id"), 10, 64)
		if err != nil {
			srv.logger.Warnf("GetNestStats: bad nest id: %v", err)
			c.JSON(http.StatusBadRequest, &APIErrorResponse{
				Error: "malformed nest ID",
			})
			return
		}

		nest := nestProcessor.GetNestByID(nestId)
		if nest == nil {
			c.JSON(http.StatusNotFound, &APIErrorResponse{
				Error: "Nest not found",
			})
			return
		}
		nests = []*models.Nest{nest}
	} else {
		nests = nestProcessor.GetNests()
	}

	stats := nestProcessor.GetStatsSnapshot()

	type APIStatsTimePeriod struct {
		StartTime       time.Time                  `json:"start_time"`
		EndTime         time.Time                  `json:"end_time"`
		DurationSeconds uint64                     `json:"duration_seconds"`
		PokemonCounts   *processor.CountsByPokemon `json:"pokemon_counts"`
	}

	type APINestStatsTimePeriods struct {
		Nest        *APINest              `json:"nest"`
		TimePeriods []*APIStatsTimePeriod `json:"time_periods"`
	}

	type APINestStats struct {
		DurationSeconds uint64                     `json:"duration_seconds"`
		NestStat        *APINestStatsTimePeriods   `json:"nest_stats,omitempty"`
		NestStats       []*APINestStatsTimePeriods `json:"nests_stats,omitempty"`
		GlobalStats     []*APIStatsTimePeriod      `json:"global_time_periods"`
	}

	type APINestStatsResponse struct {
		Stats APINestStats `json:"stats"`
	}

	globalPeriods := make([]*APIStatsTimePeriod, len(stats.CountsByTimePeriod))
	statsByNest := make([]*APINestStatsTimePeriods, len(nests))

	for nestIdx, nest := range nests {
		timePeriods := make([]*APIStatsTimePeriod, len(stats.CountsByTimePeriod))

		for idx, tpCounts := range stats.CountsByTimePeriod {
			durationSec := uint64(tpCounts.EndTime.Sub(tpCounts.StartTime) / time.Second)

			nestEntry := tpCounts.NestCounts[nest.Id]
			if nestEntry == nil {
				nestEntry = processor.NewCountsByPokemon()
			}
			timePeriods[idx] = &APIStatsTimePeriod{
				StartTime:       tpCounts.StartTime,
				EndTime:         tpCounts.EndTime,
				DurationSeconds: durationSec,
				PokemonCounts:   nestEntry,
			}
			if globalPeriods[idx] == nil {
				globalPeriods[idx] = &APIStatsTimePeriod{
					StartTime:       tpCounts.StartTime,
					EndTime:         tpCounts.EndTime,
					DurationSeconds: durationSec,
					PokemonCounts:   tpCounts.GlobalCounts,
				}
			}
		}

		statsByNest[nestIdx] = &APINestStatsTimePeriods{
			Nest:        nestToAPINest(nest, false),
			TimePeriods: timePeriods,
		}
	}

	resp := &APINestStatsResponse{
		Stats: APINestStats{
			DurationSeconds: uint64(stats.Duration / time.Second),
			GlobalStats:     globalPeriods,
		},
	}

	if nestIdStr == "" {
		resp.Stats.NestStats = statsByNest
	} else {
		resp.Stats.NestStat = statsByNest[0]
	}

	c.JSON(http.StatusOK, resp)
}
