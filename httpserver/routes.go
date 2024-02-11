package httpserver

import (
	"net/http/pprof"

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

	debugGroup := r.Group("/debug/pprof")
	debugGroup.GET("/cmdline", func(c *gin.Context) {
		pprof.Cmdline(c.Writer, c.Request)
	})
	debugGroup.GET("/heap", func(c *gin.Context) {
		pprof.Index(c.Writer, c.Request)
	})
	debugGroup.GET("/block", func(c *gin.Context) {
		pprof.Index(c.Writer, c.Request)
	})
	debugGroup.GET("/mutex", func(c *gin.Context) {
		pprof.Index(c.Writer, c.Request)
	})
	debugGroup.GET("/trace", func(c *gin.Context) {
		pprof.Trace(c.Writer, c.Request)
	})
	debugGroup.GET("/profile", func(c *gin.Context) {
		pprof.Profile(c.Writer, c.Request)
	})
	debugGroup.GET("/symbol", func(c *gin.Context) {
		pprof.Symbol(c.Writer, c.Request)
	})
}
