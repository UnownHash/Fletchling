package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/UnownHash/Fletchling/processor"
	"github.com/UnownHash/Fletchling/stats_collector"
)

func init() {
	gin.SetMode(gin.ReleaseMode)
}

type HTTPServer struct {
	logger               *logrus.Logger
	ginRouter            *gin.Engine
	nestProcessorManager *processor.NestProcessorManager
	statsCollector       stats_collector.StatsCollector
	reloadFn             func() error
}

// Run starts and runs the HTTP server until 'ctx' is cancelled or the server fails to start.
func (srv *HTTPServer) Run(ctx context.Context, address string, shutdownWaitTimeout time.Duration) error {
	httpServer := &http.Server{
		Addr:    address,
		Handler: srv.ginRouter,
	}

	doneCh := make(chan error, 1)

	go func() {
		var err error
		defer func() {
			doneCh <- err
		}()
		err = httpServer.ListenAndServe()
		if err != nil {
			if err == http.ErrServerClosed {
				err = nil
			} else {
				err = fmt.Errorf("Failed to listen and start http server: %w", err)
			}
		}
	}()

	select {
	case <-ctx.Done():
		sdCtx, sdCancelFn := context.WithTimeout(context.Background(), shutdownWaitTimeout)
		defer sdCancelFn()
		err := httpServer.Shutdown(sdCtx)
		if err != nil {
			if err == context.DeadlineExceeded {
				return errors.New("Graceful HTTP server shutdown timed out.")
			}
			return fmt.Errorf("Error during http server shutdown: %w", err)
		}
		return <-doneCh
	case err := <-doneCh:
		return err
	}
}

func NewHTTPServer(logger *logrus.Logger, nestProcessorManager *processor.NestProcessorManager, statsCollector stats_collector.StatsCollector, reloadFn func() error) (*HTTPServer, error) {
	// Create the web server.
	r := gin.New()
	r.Use(gin.RecoveryWithWriter(logger.Writer()))
	statsCollector.RegisterGinEngine(r)

	srv := &HTTPServer{
		logger:               logger,
		ginRouter:            r,
		nestProcessorManager: nestProcessorManager,
		statsCollector:       statsCollector,
		reloadFn:             reloadFn,
	}

	srv.setupRoutes()
	return srv, nil
}
