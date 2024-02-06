package koji_client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type cliProps struct {
	list   []Property
	byName map[string]*Property
	byId   map[int]*Property
}

type AdminClient struct {
	logger      *logrus.Logger
	url         string
	bearerToken string

	propsRWMutex sync.RWMutex
	propsOnce    sync.Once
	props        *cliProps

	httpClient *http.Client
}

func (cli *AdminClient) getPropertiesFromKoji() (*cliProps, error) {
	properties, err := cli.GetAllProperties()
	if err != nil {
		return nil, err
	}

	props := &cliProps{
		list:   properties,
		byName: properties.AsMapByName(),
		byId:   properties.AsMapById(),
	}

	return props, nil
}

func (cli *AdminClient) refreshPropertiesFromKoji() (*cliProps, error) {
	props, err := cli.getPropertiesFromKoji()
	if err != nil {
		return nil, err
	}

	cli.propsRWMutex.Lock()
	defer cli.propsRWMutex.Unlock()

	cli.props = props
	return props, nil
}

func (cli *AdminClient) RefreshProperties() error {
	_, err := cli.refreshPropertiesFromKoji()
	return err
}

func (cli *AdminClient) getProperties() (*cliProps, error) {
	props := func() *cliProps {
		cli.propsRWMutex.RLock()
		defer cli.propsRWMutex.RUnlock()
		return cli.props
	}()

	if props != nil {
		return props, nil
	}

	return cli.refreshPropertiesFromKoji()
}

// Returns nil, nil if property doesn't exist.
func (cli *AdminClient) GetPropertyByName(name string) (*Property, error) {
	props, err := cli.getProperties()
	if err != nil {
		return nil, err
	}
	prop, _ := props.byName[name]
	return prop, nil
}

// doesn't handle color category when creating
func (cli *AdminClient) GetOrCreateProperty(name string, value any) (*Property, error) {
	prop, err := cli.GetPropertyByName(name)
	if prop != nil || err != nil {
		return prop, err
	}

	var mapping = map[reflect.Kind]string{
		reflect.String:  "string",
		reflect.Bool:    "boolean",
		reflect.Map:     "object",
		reflect.Slice:   "array",
		reflect.Int8:    "number",
		reflect.Int16:   "number",
		reflect.Int32:   "number",
		reflect.Int64:   "number",
		reflect.Uint8:   "number",
		reflect.Uint16:  "number",
		reflect.Uint32:  "number",
		reflect.Uint64:  "number",
		reflect.Float32: "number",
		reflect.Float64: "number",
	}

	category := mapping[reflect.ValueOf(value).Kind()]
	if category == "" {
		return nil, fmt.Errorf("couldn't determine type of value (%T) for property '%s'", value, name)
	}

	property := &Property{
		Name:     name,
		Category: category,
	}

	property, err = cli.CreateProperty(property)
	if err != nil {
		return nil, err
	}

	cli.refreshPropertiesFromKoji()
	return property, nil
}

func (cli *AdminClient) doOneRequest(method, url_str string, reader *bytes.Reader) (*http.Response, error) {
	var io_reader io.Reader

	if reader != nil {
		reader.Seek(0, io.SeekStart)
		io_reader = reader
	}

	req, err := http.NewRequest(method, cli.url+url_str, io_reader)
	if err != nil {
		return nil, fmt.Errorf("error forming http request: %s", err)
	}

	req_hdr := req.Header
	req_hdr.Set("Content-Type", "application/json")

	resp, err := cli.httpClient.Do(req)
	if err != nil {
		// this shouldn't happen.. fail the whole thing.
		return nil, fmt.Errorf("error doing http request: %s", err)
	}

	return resp, nil
}

func (cli *AdminClient) login() error {
	type loginRequest struct {
		Password string `json:"password"`
	}

	login_req := loginRequest{Password: cli.bearerToken}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	err := encoder.Encode(login_req)
	if err != nil {
		return fmt.Errorf("error encoding login request: %s", err)
	}

	resp, err := cli.doOneRequest("POST", "/config/login", bytes.NewReader(buf.Bytes()))
	if err != nil {
		return fmt.Errorf("error doing http request: %s", err)
	}

	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != 200 {
		return fmt.Errorf("login http request returned non-200 code '%d'", resp.StatusCode)
	}

	return nil
}

func (cli *AdminClient) makeInternalRequest(method, url_str string, reader *bytes.Reader) (*http.Response, error) {
	resp, err := cli.doOneRequest(method, "/internal/admin"+url_str, reader)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		return resp, err
	}

	cli.logger.Warnf("request to '%s' not authorized, trying to login..", url_str)

	// we're going to make a new request. clean this
	// one up
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	// login and try again
	if err := cli.login(); err != nil {
		return nil, err
	}

	return cli.doOneRequest(method, url_str, reader)
}

func (cli *AdminClient) CreateGeofence(geofence *Geofence) (*Geofence, error) {
	var buf bytes.Buffer

	if geofence.Id != 0 {
		return nil, errors.New("geofence has an id already")
	}

	if len(geofence.Name) == 0 {
		return nil, errors.New("geofence has no name")
	}

	{
		// https://github.com/TurtIeSocks/Koji/issues/220
		looksLikeNumber := true
		for _, r := range geofence.Name {
			// throwing in .-+ to be extra safe.
			if (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '+' {
				continue
			}
			looksLikeNumber = false
			break
		}

		if looksLikeNumber {
			return nil, errors.New("geofence name looks like a number and this would go poorly. pick a better name.")
		}
	}

	now := time.Now()

	if geofence.CreatedAt == nil {
		geofence.CreatedAt = &now
	}

	if geofence.UpdatedAt == nil {
		geofence.UpdatedAt = &now
	}

	encoder := json.NewEncoder(&buf)
	err := encoder.Encode(geofence)
	if err != nil {
		return nil, err
	}

	resp, err := cli.makeInternalRequest("POST", "/geofence/", bytes.NewReader(buf.Bytes()))
	if err != nil {
		return nil, err
	}

	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	var kojiResp Geofence

	if err := decodeResponse(resp, &kojiResp); err != nil {
		return nil, err
	}

	// POST seems to not give properties back, so, let's try to give a complete object
	return cli.GetGeofenceById(kojiResp.Id)
}

// untested, unused
func (cli *AdminClient) UpdateGeofence(geofence *Geofence) (*Geofence, error) {
	var buf bytes.Buffer

	now := time.Now()
	if geofence.UpdatedAt == nil {
		geofence.UpdatedAt = &now
	}

	encoder := json.NewEncoder(&buf)
	err := encoder.Encode(geofence)
	if err != nil {
		return nil, err
	}

	resp, err := cli.makeInternalRequest("PATCH", "/geofence/"+strconv.Itoa(geofence.Id)+"/", bytes.NewReader(buf.Bytes()))
	if err != nil {
		return nil, err
	}

	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	//var kojiResp GeofenceResponse
	// we're just going to re-query, so don't bother really unmarshalling.
	var kojiResp json.RawMessage

	if err := decodeResponse(resp, &kojiResp); err != nil {
		return nil, err
	}

	return cli.GetGeofenceById(geofence.Id)
}

func (cli *AdminClient) GetGeofenceById(id int) (*Geofence, error) {
	resp, err := cli.makeInternalRequest("GET", "/geofence/"+strconv.Itoa(id)+"/", nil)
	if err != nil {
		return nil, err
	}

	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	var kojiResp Geofence

	if err := decodeResponse(resp, &kojiResp); err != nil {
		return nil, err
	}
	return &kojiResp, err
}

func (cli *AdminClient) GetAllGeofences() ([]*GeofenceBrief, error) {
	resp, err := cli.makeInternalRequest("GET", "/geofence/all/", nil)
	if err != nil {
		return nil, err
	}

	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	var kojiResp []*GeofenceBrief

	if err := decodeResponse(resp, &kojiResp); err != nil {
		return nil, err
	}

	return kojiResp, err
}

func (cli *AdminClient) GetAllGeofencesFull() ([]*Geofence, error) {
	bGeofences, err := cli.GetAllGeofences()
	if err != nil {
		return nil, err
	}

	geofences := make([]*Geofence, len(bGeofences))

	for idx, bGeofence := range bGeofences {
		geofence, err := cli.GetGeofenceById(bGeofence.Id)
		if err != nil {
			return nil, err
		}
		geofences[idx] = geofence
	}

	return geofences, nil
}

func (cli *AdminClient) CreateProperty(property *Property) (*Property, error) {
	var buf bytes.Buffer

	now := time.Now()
	if property.CreatedAt == nil {
		property.CreatedAt = &now
	}
	if property.UpdatedAt == nil {
		property.UpdatedAt = &now
	}

	encoder := json.NewEncoder(&buf)
	err := encoder.Encode(property)
	if err != nil {
		return nil, err
	}

	resp, err := cli.makeInternalRequest("POST", "/property/", bytes.NewReader(buf.Bytes()))
	if err != nil {
		return nil, err
	}

	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	var kojiResp Property

	if err := decodeResponse(resp, &kojiResp); err != nil {
		return nil, err
	}

	return &kojiResp, nil
}

func (cli *AdminClient) GetAllProperties() (Properties, error) {
	resp, err := cli.makeInternalRequest("GET", "/property/all/", nil)
	if err != nil {
		return nil, err
	}

	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	var kojiResp Properties

	if err := decodeResponse(resp, &kojiResp); err != nil {
		return nil, err
	}

	return kojiResp, err
}

func (cli *AdminClient) GetAllProjects() ([]ProjectBrief, error) {
	resp, err := cli.makeInternalRequest("GET", "/project/all/", nil)
	if err != nil {
		return nil, err
	}

	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	var kojiResp []ProjectBrief

	if err := decodeResponse(resp, &kojiResp); err != nil {
		return nil, err
	}
	return kojiResp, err
}

func (cli *AdminClient) GetProjectByID(id int) (*Project, error) {
	resp, err := cli.makeInternalRequest("GET", "/project/"+strconv.Itoa(id)+"/", nil)
	if err != nil {
		return nil, err
	}

	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	var kojiResp Project

	if err := decodeResponse(resp, &kojiResp); err != nil {
		return nil, err
	}
	return &kojiResp, err
}

func (cli *AdminClient) GetProjectByName(name string) (*Project, error) {
	projects, err := cli.GetAllProjects()
	if err != nil {
		return nil, err
	}

	for _, project := range projects {
		if project.Name == name {
			return cli.GetProjectByID(project.Id)
		}
	}

	return nil, fmt.Errorf("No project was found with name '%s'", name)
}

func NewAdminClient(logger *logrus.Logger, urlStr, bearerToken string) (*AdminClient, error) {
	_, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("Invalid Koji URL: %s", urlStr)
	}

	cookie_jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar")
	}

	cli := &AdminClient{
		logger:      logger,
		url:         urlStr,
		bearerToken: bearerToken,
		httpClient: &http.Client{
			Jar: cookie_jar,
		},
	}
	err = cli.login()
	if err != nil {
		return nil, err
	}
	return cli, nil
}
