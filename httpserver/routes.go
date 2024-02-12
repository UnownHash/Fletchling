package httpserver

import (
	"net/http"
	"net/http/pprof"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
)

func (srv *HTTPServer) authorizeAPI(c *gin.Context) {
	// anything goes for now.
	c.Next()
}

func (srv *HTTPServer) setupRoutes() {
	r := srv.ginRouter

	r.Use(gin.RecoveryWithWriter(srv.logger.Writer()))

	r.POST("/webhook", srv.handleWebhook)

	apiGroup := r.Group("/api", srv.authorizeAPI)

	configGroup := apiGroup.Group("/config")
	configGroup.GET("", srv.handleGetConfig)
	configGroup.GET("/reload", srv.handleReload)
	configGroup.PUT("/reload", srv.handleReload)

	nestsGroup := apiGroup.Group("/nests")
	nestsGroup.GET("", srv.handleGetNests)
	nestsGroup.GET("/_/stats", srv.handleGetNestStats)
	nestsGroup.GET("/:nest_id", srv.handleGetNest)
	nestsGroup.GET("/:nest_id/stats", srv.handleGetNestStats)

	statsGroup := apiGroup.Group("/stats/")
	statsGroup.PUT("/purge/all", srv.handlePurgeAllStats)
	statsGroup.PUT("/purge/keep", srv.handlePurgeKeepStats)
	statsGroup.PUT("/purge/oldest", srv.handlePurgeOldestStats)
	statsGroup.PUT("/purge/newest", srv.handlePurgeNewestStats)

	debugGroup := r.Group("/debug")

	// run the GC. I guess this is also available via '/debug/pprof/heap?gc=1"
	debugGroup.PUT("/gc/flush", func(c *gin.Context) {
		now := time.Now()
		srv.logger.Info("/gc/flush: starting garbage collector")
		runtime.GC()
		duration := time.Now().Sub(now).Truncate(time.Millisecond)
		srv.logger.Infof("/gc/flush: garbage collector ran in %s", duration)
		resp := struct {
			Message    string `json:"message"`
			DurationMs int64  `json:"duration_ms"`
		}{"garbage collector has been run.", duration.Milliseconds()}
		c.JSON(http.StatusOK, &resp)
	})

	// run the GC and return as much memory to OS as possible.
	debugGroup.PUT("/gc/free", func(c *gin.Context) {
		now := time.Now()
		srv.logger.Info("/gc/free: starting garbage collector and memory freeing")
		debug.FreeOSMemory()
		duration := time.Now().Sub(now).Truncate(time.Millisecond)
		srv.logger.Infof("/gc/free: garbage collector and memory freeing ran in %s", duration)
		resp := struct {
			Message    string `json:"message"`
			DurationMs int64  `json:"duration_ms"`
		}{"garbage collector has been run and memory freed.", duration.Milliseconds()}
		c.JSON(http.StatusOK, &resp)
	})

	pprofGroup := debugGroup.Group("/pprof")
	pprofGroup.GET("/cmdline", func(c *gin.Context) {
		pprof.Cmdline(c.Writer, c.Request)
	})
	pprofGroup.GET("/heap", func(c *gin.Context) {
		pprof.Index(c.Writer, c.Request)
	})
	pprofGroup.GET("/block", func(c *gin.Context) {
		pprof.Index(c.Writer, c.Request)
	})
	pprofGroup.GET("/mutex", func(c *gin.Context) {
		pprof.Index(c.Writer, c.Request)
	})
	pprofGroup.GET("/trace", func(c *gin.Context) {
		pprof.Trace(c.Writer, c.Request)
	})
	pprofGroup.GET("/profile", func(c *gin.Context) {
		pprof.Profile(c.Writer, c.Request)
	})
	pprofGroup.GET("/symbol", func(c *gin.Context) {
		pprof.Symbol(c.Writer, c.Request)
	})
}
