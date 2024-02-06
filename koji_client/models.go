package koji_client

import (
	"time"

	"github.com/paulmach/orb/geojson"
)

var jsonNull = []byte("null")

type RouteBrief struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type Route struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type GeofenceBrief struct {
	Id      int    `json:"id"`
	Name    string `json:"name"`
	GeoType string `json:"geo_type"`
	Mode    string `json:"mode"`
	Parent  *int   `json:"parent,omitempty"`
}

type Property struct {
	Name         string     `json:"name"`
	Category     string     `json:"category"`
	DefaultValue any        `json:"default_value"`
	PropertyId   int        `json:"id"`
	CreatedAt    *time.Time `json:"created_at,omitempty"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
}

type Properties []Property

func (p Properties) AsMapById() map[int]*Property {
	m := make(map[int]*Property)
	for i := range p {
		m[p[i].PropertyId] = &p[i]
	}
	return m
}

func (p Properties) AsMapByName() map[string]*Property {
	m := make(map[string]*Property)
	for i := range p {
		m[p[i].Name] = &p[i]
	}
	return m
}

type GeofenceProperty struct {
	PropertyId int    `json:"property_id"`
	Name       string `json:"name"`
	Value      any    `json:"value"`
}

type Geofence struct {
	Id      int    `json:"id"`
	Name    string `json:"name"`
	GeoType string `json:"geo_type"`
	Mode    string `json:"mode"`
	Parent  *int   `json:"parent,omitempty"`

	CreatedAt  *time.Time         `json:"created_at,omitempty"`
	UpdatedAt  *time.Time         `json:"updated_at,omitempty"`
	Geometry   *geojson.Geometry  `json:"geometry,omitempty"`
	Properties []GeofenceProperty `json:"properties,omitempty"`
	Routes     []RouteBrief       `json:"routes,omitempty"`
	Projects   []int              `json:"projects,omitempty"`
}

type ProjectBrief struct {
	Id        int    `json:"id"`
	Name      string `json:"name"`
	Geofences []int  `json:"geofences"`
}

type Project struct {
	Id        int    `json:"id"`
	Name      string `json:"name"`
	Geofences []int  `json:"geofences"`

	CreatedAt   time.Time `json:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
	Description *string   `json:"description"`
	Scanner     bool      `json:"scanner"`
	APIEndpoint *string   `json:"api_endpoint,omitempty"`
	APIKey      *string   `json:"api_key,omitempty"`
}
