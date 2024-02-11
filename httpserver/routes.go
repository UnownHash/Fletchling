package httpserver

import (
	"net/http"
	"net/http/pprof"
	"runtime"
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
	debugGroup.PUT("/gc-flush", func(c *gin.Context) {
		now := time.Now()
		runtime.GC()
		resp := struct {
			Message    string `json:"message"`
			DurationMs int64  `json:"duration_ms"`
		}{"garbage collector has been run", time.Now().Sub(now).Milliseconds()}
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
