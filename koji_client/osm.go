package koji_client

import (
	"errors"
	"fmt"

	"github.com/paulmach/orb/geojson"
	"github.com/paulmach/osm"
	"github.com/paulmach/osm/osmgeojson"
)

func (cli *AdminClient) OSMToGeofences(osm_data *osm.OSM, parent *Geofence, projects []int, createProperties bool) ([]*Geofence, error) {
	fc, err := osmgeojson.Convert(osm_data)
	if err != nil {
		return nil, fmt.Errorf("error converting osm to geojson: %w", err)
	}

	if len(fc.Features) == 0 {
		return nil, nil
	}

	var parent_id *int
	if parent != nil {
		parent_id = &parent.Id
	}

	geofences := make([]*Geofence, 0, len(fc.Features))

	for _, feature := range fc.Features {
		geofence, err := cli.GeofenceFromOSMFeature(feature, parent_id, projects, createProperties)
		if err != nil {
			cli.logger.Warnf("skipping geofence: %v", err)
			continue
		}
		geofences = append(geofences, geofence)
	}

	return geofences, nil
}

func (cli *AdminClient) GeofenceFromOSMFeature(feature *geojson.Feature, parent_id *int, projects []int, createProperties bool) (*Geofence, error) {
	// usually the name is in tags.

	var tagsIface map[string]any
	var haveTagsIface bool

	name, _ := feature.Properties["name"].(string)
	tagsStr, haveTags := feature.Properties["tags"].(map[string]string)
	if !haveTags {
		tagsIface, haveTagsIface = feature.Properties["tags"].(map[string]any)
	}
	if name == "" {
		if haveTags {
			name, _ = tagsStr["name"]
		} else if haveTagsIface {
			name, _ = tagsIface["name"].(string)
		}
		if name == "" {
			return nil, errors.New("no name in properties nor tags")
		}
	}

	seenProperties := make(map[string]struct{})
	properties := make([]GeofenceProperty, 0)
	for k, v := range feature.Properties {
		if k == "tags" {
			// these are folded in below.
			continue
		}

		// skip meta and relations because they are object
		// and array respectively and seem empty.
		if k == "meta" || k == "relations" {
			continue
		}

		var prop *Property
		var err error
		if !createProperties || v == nil {
			prop, err = cli.GetPropertyByName(k)
		} else {
			prop, err = cli.GetOrCreateProperty(k, v)
		}
		if err != nil {
			return nil, err
		}
		if prop == nil {
			continue
		}
		properties = append(properties,
			GeofenceProperty{
				PropertyId: prop.PropertyId,
				Name:       k,
				Value:      v,
			},
		)
		seenProperties[k] = struct{}{}
	}
	if haveTags {
		for k, v := range tagsStr {
			if _, ok := seenProperties[k]; ok {
				continue
			}

			var prop *Property
			var err error

			if createProperties {
				prop, err = cli.GetOrCreateProperty(k, v)
			} else {
				prop, err = cli.GetPropertyByName(k)
			}
			if err != nil {
				return nil, err
			}
			if prop == nil {
				continue
			}

			properties = append(properties,
				GeofenceProperty{
					PropertyId: prop.PropertyId,
					Name:       k,
					Value:      v,
				},
			)
		}
	}
	if haveTagsIface {
		for k, v := range tagsIface {
			if _, ok := seenProperties[k]; ok {
				continue
			}
			var prop *Property
			var err error

			if !createProperties || v == nil {
				prop, err = cli.GetPropertyByName(k)
			} else {
				prop, err = cli.GetOrCreateProperty(k, v)
			}
			if err != nil {
				return nil, err
			}
			if prop == nil {
				continue
			}
			properties = append(properties,
				GeofenceProperty{
					PropertyId: prop.PropertyId,
					Name:       k,
					Value:      v,
				},
			)
		}
	}

	geometry := geojson.NewGeometry(feature.Geometry)

	return &Geofence{
		Name:       name,
		GeoType:    geometry.Type,
		Mode:       "unset",
		Parent:     parent_id,
		Geometry:   geometry,
		Projects:   projects,
		Properties: properties,
	}, nil
}
