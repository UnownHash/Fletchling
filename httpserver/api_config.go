package httpserver

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/UnownHash/Fletchling/processor"
)

func (srv *HTTPServer) handleReload(c *gin.Context) {
	type reloadResponse struct {
		Message string `json:"message"`
	}

	err := srv.reloadFn()
	if err != nil {
		srv.logger.Error(err)
		c.JSON(http.StatusInternalServerError, APIErrorResponse{
			Error: "an internal error occurred: check the logs",
		})
		return
	}

	srv.logger.Infof("processor config reloaded")

	c.JSON(http.StatusOK, reloadResponse{
		Message: "config has been reloaded",
	})
}

func (srv *HTTPServer) handleGetConfig(c *gin.Context) {
	type configResponse struct {
		ProcessorConfig processor.Config `json:"processor"`
	}

	type getConfigResponse struct {
		Config configResponse `json:"config"`
	}

	var resp getConfigResponse
	resp.Config.ProcessorConfig = srv.nestProcessorManager.GetConfig()

	c.JSON(http.StatusOK, resp)
}
