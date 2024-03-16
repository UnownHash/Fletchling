package httpserver

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/UnownHash/Fletchling/filters"
	"github.com/UnownHash/Fletchling/processor"
	"github.com/UnownHash/Fletchling/version"
)

func (srv *HTTPServer) doDBRefresh(c *gin.Context, allSpawnpoints bool) error {
	filtersConfig := srv.filtersConfigFn()
	concurrency := filtersConfig.Concurrency

	if concurrencyStr := c.Query("concurrency"); concurrencyStr != "" {
		pConcurrency, err := strconv.ParseInt(concurrencyStr, 10, 32)
		if err == nil && pConcurrency > 0 {
			concurrency = int(pConcurrency)
		} else {
			if err == nil {
				err = errors.New("must be positive")
			}
			srv.logger.Warnf("ignoring invalid concurrency param '%s': %s", concurrencyStr, err)
		}
	}

	ctx := c.Request.Context()

	refreshConfig := filters.RefreshNestConfig{
		FiltersConfig:           filtersConfig,
		Concurrency:             concurrency,
		ForceSpawnpointsRefresh: allSpawnpoints,
	}

	srv.logger.Infof("starting nest refresh")
	err := srv.dbRefresher.RefreshAllNests(ctx, refreshConfig)
	if err != nil {
		return err
	}
	srv.logger.Infof("finished nest refresh")
	return nil
}

func (srv *HTTPServer) handleReload(c *gin.Context) {
	allSpawnpoints := c.Query("spawnpoints") == "all"

	if allSpawnpoints || c.Query("refresh") == "1" {
		err := srv.doDBRefresh(c, allSpawnpoints)
		if err != nil {
			srv.logger.Errorf("failed to refresh nests: %v", err)
			c.JSON(http.StatusInternalServerError, APIErrorResponse{
				Error: "an internal error occurred: check the logs",
			})
			return
		}
	}

	type reloadResponse struct {
		Message string `json:"message"`
	}

	srv.logger.Infof("reloading config")

	err := srv.reloadFn()
	if err != nil {
		srv.logger.Error(err)
		c.JSON(http.StatusInternalServerError, APIErrorResponse{
			Error: "an internal error occurred: check the logs",
		})
		return
	}

	srv.logger.Infof("config reloaded")

	c.JSON(http.StatusOK, reloadResponse{
		Message: "config has been reloaded",
	})
}

func (srv *HTTPServer) handleGetConfig(c *gin.Context) {
	type configResponse struct {
		ProcessorConfig processor.Config `json:"processor"`
	}

	type getConfigResponse struct {
		Config  configResponse `json:"config"`
		Version string         `json:"version"`
	}

	resp := getConfigResponse{
		Version: version.APP_VERSION,
		Config: configResponse{
			ProcessorConfig: srv.nestProcessorManager.GetConfig(),
		},
	}

	c.JSON(http.StatusOK, resp)
}
