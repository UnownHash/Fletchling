package koji_client

import (
	"github.com/sirupsen/logrus"
)

type Client struct {
	*AdminClient
	*APIClient
}

func NewClient(logger *logrus.Logger, urlStr, bearerToken string) (*Client, error) {
	adminCli, err := NewAdminClient(logger, urlStr, bearerToken)
	if err != nil {
		return nil, err
	}
	apiCli, err := NewAPIClient(logger, urlStr, bearerToken)
	if err != nil {
		return nil, err
	}
	// sharing is caring.
	apiCli.httpClient = adminCli.httpClient
	return &Client{adminCli, apiCli}, nil
}
