package geo

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geojson"
)

type GeofencesFileEntry struct {
	Name string         `json:"name"`
	Path orb.LineString `json:"path"`
}

// LoadFeaturesFromFile will load the []{name:,path:} type of file or
// a FeatureCollection.
func LoadFeaturesFromFile(filename string) ([]*geojson.Feature, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("could not open '%s': %w", filename, err)
	}
	defer f.Close()

	contents, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("error during read from '%s': %w", filename, err)
	}

	for len(contents) > 0 && contents[0] == ' ' || contents[0] == '\t' {
		contents = contents[1:]
	}

	switch contents[0] {
	case '{':
		var featureCollection *geojson.FeatureCollection

		err := json.Unmarshal(contents, &featureCollection)
		if err != nil || featureCollection == nil {
			if err == nil {
				err = errors.New("format not supported")
			}
			return nil, fmt.Errorf("couldn't decode '%s' as a FeatureCollection: %v", filename, err)
		}

		return featureCollection.Features, nil
	case '[':
		var geofences []GeofencesFileEntry

		if err := json.Unmarshal(contents, &geofences); err != nil {
			return nil, fmt.Errorf("couldn't decode '%s' as a name/path json file: %v", filename, err)
		}

		features := make([]*geojson.Feature, len(geofences))
		idx := 0

		for _, geofence := range geofences {
			if geofence.Name == "" {
				return nil, fmt.Errorf("geofence in '%s' is missing name", filename)
			}

			l := len(geofence.Path)
			if l < 2 {
				return nil, fmt.Errorf("geofence in '%s' has bad path", filename)
			}

			if geofence.Path[0] != geofence.Path[l-1] {
				geofence.Path = append(geofence.Path, geofence.Path[0])
			}

			feature := geojson.NewFeature(
				orb.Polygon(
					[]orb.Ring{
						orb.Ring(geofence.Path),
					},
				),
			)
			feature.Properties["name"] = geofence.Name
			features[idx] = feature
			idx++
		}

		features = features[:idx]

		return features, nil
	}

	return nil, fmt.Errorf("couldn't decode '%s': format unsupported", filename)
}
