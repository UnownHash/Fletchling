package geo

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geojson"
)

type GeofencesFileEntry struct {
	Name string         `json:"name"`
	Path orb.LineString `json:"path"`
}

func LoadGeofencesFile(filename string) ([]*geojson.Feature, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("--overpass-src should be 'koji' or a filename: %v", err)
	}
	defer f.Close()

	var geofences []GeofencesFileEntry

	decoder := json.NewDecoder(f)
	if err := decoder.Decode(&geofences); err != nil {
		return nil, fmt.Errorf("'%s' cannot be loaded: bad json: %v", filename, err)
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
