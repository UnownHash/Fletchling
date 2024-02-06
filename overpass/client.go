package overpass

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"time"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geo"
	"github.com/paulmach/osm"
	"github.com/sirupsen/logrus"

	"github.com/UnownHash/Fletchling/util"
)

const (
	//DEFAULT_URL = "https://overpass.kumi.systems/api/interpreter"
	DEFAULT_URL = "https://overpass-api.de/api/interpreter"
)

var (
	dupeQueryBytes = []byte("Dispatcher_Client::request_read_and_idx::duplicate_query")
)

type Client struct {
	logger     *logrus.Logger
	apiUrl     string
	httpClient *http.Client
}

func (cli *Client) doSingleQuery(v url.Values) (*osm.OSM, error) {
	resp, err := cli.httpClient.PostForm(cli.apiUrl, v)
	if err != nil {
		return nil, err
	}

	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	var osm_data osm.OSM

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		err = matchBodyAgainstErrors(respBytes)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("received status code %d: body: %s", resp.StatusCode, string(respBytes))
	}

	err = json.Unmarshal(respBytes, &osm_data)
	if err != nil {
		if nerr := matchBodyAgainstErrors(respBytes); nerr != nil {
			return nil, nerr
		}
		return nil, err
	}

	return &osm_data, nil
}

func (cli *Client) fuzzBound(bound orb.Bound) (orb.Bound, string) {
	randomMeters := rand.Intn(5 * 1000)
	bound = geo.BoundPad(bound, float64(randomMeters))
	return bound, fmt.Sprintf("%f,%f,%f,%f", bound.Min[1], bound.Min[0], bound.Max[1], bound.Max[0])
}

func (cli *Client) GetPossibleNestLocations(ctx context.Context, bound orb.Bound) (*osm.OSM, error) {
	bound, bbox := cli.fuzzBound(bound)
	urlValues := url.Values{
		"data": {
			searchPrefix + bbox + searchSuffix,
		},
	}

	max_tries := 5

	for {
		osm_data, err := cli.doSingleQuery(urlValues)
		if err == nil {
			return osm_data, nil
		}
		if err == errTimeout {
			cli.logger.Warnf("received timeout. sleeping 1 second.")
			if err := util.SleepContext(ctx, time.Second); err != nil {
				return nil, err
			}
			continue
		}
		if err == errDupeQuery {
			if max_tries <= 0 {
				return nil, err
			}
			bound, bbox = cli.fuzzBound(bound)
			urlValues["data"][0] = searchPrefix + bbox + searchSuffix
			max_tries--
			continue
		}
		return nil, err
	}
}

func NewClient(logger *logrus.Logger, apiUrl string) (*Client, error) {
	if logger == nil {
		return nil, errors.New("No logger given")
	}
	if apiUrl == "" {
		return nil, errors.New("No apiUrl given")
	}
	return &Client{
		logger:     logger,
		apiUrl:     apiUrl,
		httpClient: &http.Client{},
	}, nil
}

var searchPrefix = `[out:json]
[timeout:100000]
[bbox:`
var searchSuffix = `];
(
    way[leisure=park];
    way[landuse=recreation_ground];
    way[leisure=recreation_ground];
    way[leisure=pitch];
    way[leisure=garden];
    way[leisure=golf_course];
    way[leisure=playground];
    way[landuse=meadow];
    way[landuse=grass];
    way[landuse=greenfield];
    way[natural=scrub];
    way[natural=heath];
    way[natural=grassland];
    way[landuse=farmyard];
    way[landuse=vineyard];
    way[landuse=farmland];
    way[landuse=orchard];
    way[natural=plateau];
    way[natural=moor];
    way["leisure"="nature_reserve"];
    
    rel[leisure=park];
    rel[landuse=recreation_ground];
    rel[leisure=recreation_ground];
    rel[leisure=pitch];
    rel[leisure=garden];
    rel[leisure=golf_course];
    rel[leisure=playground];
    rel[landuse=meadow];
    rel[landuse=grass];
    rel[landuse=greenfield];
    rel[natural=scrub];
    rel[natural=heath];
    rel[natural=grassland];
    rel[landuse=farmyard];
    rel[landuse=vineyard];
    rel[landuse=farmland];
    rel[landuse=orchard];
    rel[natural=plateau];
    rel[natural=moor];
    rel["leisure"="nature_reserve"];
);
out body;
>;
out skel qt;
`
