package httpserver

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type purgeResponse struct {
	TimePeriods     int `json:"time_periods"`
	DurationMinutes int `json:"duration_minutes"`
}

type purgeNewestRequest struct {
	DurationMinutes int  `json:"duration_minutes"`
	IncludeCurrent  bool `json:"include_current"`
}

type purgeKeepRequest struct {
	DurationMinutes int `json:"duration_minutes"`
}

type purgeOldestRequest struct {
	DurationMinutes int `json:"duration_minutes"`
}

func (srv *HTTPServer) handlePurgeAllStats(c *gin.Context) {
	processor := srv.nestProcessorManager.GetNestProcessor()
	timePeriods, duration := processor.KeepRecentStats(0)

	resp := &purgeResponse{
		TimePeriods:     timePeriods,
		DurationMinutes: int(duration / time.Minute),
	}

	c.JSON(http.StatusOK, resp)
}

func (srv *HTTPServer) handlePurgeKeepStats(c *gin.Context) {
	var request purgeKeepRequest

	if err := c.BindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, &APIErrorResponse{"bad request json"})
		return
	}

	if request.DurationMinutes <= 0 {
		c.JSON(http.StatusBadRequest, &APIErrorResponse{"duration_minutes should be > 0"})
		return
	}

	dur := time.Minute * time.Duration(request.DurationMinutes)

	processor := srv.nestProcessorManager.GetNestProcessor()
	timePeriods, duration := processor.KeepRecentStats(dur)

	resp := &purgeResponse{
		TimePeriods:     timePeriods,
		DurationMinutes: int(duration / time.Minute),
	}

	c.JSON(http.StatusOK, resp)
}

func (srv *HTTPServer) handlePurgeOldestStats(c *gin.Context) {
	var request purgeOldestRequest

	if err := c.BindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, &APIErrorResponse{"bad request json"})
		return
	}

	if request.DurationMinutes <= 0 {
		c.JSON(http.StatusBadRequest, &APIErrorResponse{"duration_minutes should be > 0"})
		return
	}

	dur := time.Minute * time.Duration(request.DurationMinutes)

	processor := srv.nestProcessorManager.GetNestProcessor()
	timePeriods, duration := processor.PurgeOldestStats(dur)

	resp := &purgeResponse{
		TimePeriods:     timePeriods,
		DurationMinutes: int(duration / time.Minute),
	}

	c.JSON(http.StatusOK, resp)
}

func (srv *HTTPServer) handlePurgeNewestStats(c *gin.Context) {
	var request purgeNewestRequest

	if err := c.BindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, &APIErrorResponse{"bad request json"})
		return
	}

	if request.DurationMinutes <= 0 {
		c.JSON(http.StatusBadRequest, &APIErrorResponse{"duration_minutes should be > 0"})
		return
	}

	dur := time.Minute * time.Duration(request.DurationMinutes)

	processor := srv.nestProcessorManager.GetNestProcessor()
	timePeriods, duration := processor.PurgeNewestStats(dur, request.IncludeCurrent)

	resp := &purgeResponse{
		TimePeriods:     timePeriods,
		DurationMinutes: int(duration / time.Minute),
	}

	c.JSON(http.StatusOK, resp)
}
