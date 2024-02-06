package koji_client

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type KojiResponse struct {
	Data       any    `json:"data"`
	Message    string `json:"message"`
	Status     string `json:"status"`
	StatusCode int    `json:"status_code"`
	Stats      any    `json:"stats"`
}

// returns whether or not the json decode succeeded along with an error.
// you can have a succesful json decode with an error.
func decodeResponse(resp *http.Response, obj any) error {
	kojiResp := KojiResponse{
		Data: obj,
	}

	decoder := json.NewDecoder(resp.Body)
	err := decoder.Decode(&kojiResp)
	if resp.StatusCode < 200 || resp.StatusCode > 202 {
		if kojiResp.Message == "" {
			kojiResp.Message = "<no message>"
		}
		return fmt.Errorf(
			"Received non-20[0,1,2] http status code from koji: %d %s -- %s",
			resp.StatusCode,
			resp.Status,
			kojiResp.Message,
		)
	}
	return err
}
