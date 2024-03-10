package koji_client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/paulmach/orb/geojson"
	"github.com/sirupsen/logrus"
)

/*
type FeatureCollectionResponse struct {
	Data geojson.FeatureCollection `json:"data"`
	KojiBaseResponse
}
*/

type APIClient struct {
	logger      *logrus.Logger
	url         string
	bearerToken string

	httpClient *http.Client
}

func (cli *APIClient) makePublicRequest(ctx context.Context, method, url_str string, reader *bytes.Reader) (*http.Response, error) {
	var io_reader io.Reader

	if reader != nil {
		reader.Seek(0, io.SeekStart)
		io_reader = reader
	}

	req, err := http.NewRequest(method, cli.url+url_str, io_reader)
	if err != nil {
		return nil, fmt.Errorf("error forming http request: %w", err)
	}

	req = req.WithContext(ctx)

	req_hdr := req.Header
	req_hdr.Set("Content-Type", "application/json")
	if cli.bearerToken != "" {
		req_hdr.Set("Authorization", "Bearer "+cli.bearerToken)
	}

	resp, err := cli.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error doing http request: %w", err)
	}

	return resp, nil
}

func (cli *APIClient) GetFeatureCollection(ctx context.Context, project string) (*geojson.FeatureCollection, error) {
	resp, err := cli.makePublicRequest(ctx, "GET", "/geofence/feature-collection/"+project, nil)
	if err != nil {
		return nil, err
	}

	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	var kojiResp geojson.FeatureCollection

	if err := decodeResponse(resp, &kojiResp); err != nil {
		return nil, err
	}

	return &kojiResp, err
}

func NewAPIClient(logger *logrus.Logger, urlStr, bearerToken string) (*APIClient, error) {
	_, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("Invalid Koji URL: %s", urlStr)
	}
	cli := &APIClient{
		logger:      logger,
		url:         urlStr + "/api/v1",
		bearerToken: bearerToken,
		httpClient:  &http.Client{},
	}
	return cli, nil
}
