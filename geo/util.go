package geo

import (
	"fmt"
	"strconv"

	"github.com/paulmach/orb/geojson"
	"gopkg.in/guregu/null.v4"
)

func idIsValid(id any) (int64, error) {
	var nestId int64

	switch v := id.(type) {
	case string:
		var err error
		nestId, err = strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("id '%s' can't be parsed as int", v)
		}
	case int:
		nestId = int64(v)
	case uint:
		nestId = int64(v)
	case int64:
		nestId = v
	case uint64:
		nestId = int64(v)
	default:
		return 0, fmt.Errorf("id '%v' type '%T' not supported", v, v)
	}

	return nestId, nil
}

func NameAndIntIdFromFeature(feature *geojson.Feature) (string, null.String, int64, error) {
	var areaName null.String

	props := feature.Properties

	name, ok := props["name"].(string)
	if !ok {
		return "<unknown>", areaName, 0, fmt.Errorf("feature has no name")
	}

	fullName := name

	if parent, _ := props["parent"].(string); parent != "" {
		areaName = null.StringFrom(parent)
		fullName = parent + "/" + name
	}

	id, ok := props["id"]
	if !ok {
		return name, areaName, 0, fmt.Errorf("feature '%s' has no id", fullName)
	}

	nestId, err := idIsValid(id)
	if err != nil {
		return name, areaName, 0, fmt.Errorf("feature '%s': %s", fullName, err)
	}

	return name, areaName, nestId, nil
}
