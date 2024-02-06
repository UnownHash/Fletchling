package overpass

import (
	"strconv"

	"github.com/paulmach/orb/geojson"
)

func AdjustFeatureProperties(feature *geojson.Feature) {
	props := feature.Properties

	var tagsIface map[string]any
	var haveTagsIface bool

	name, _ := props["name"].(string)
	tagsStr, haveTags := props["tags"].(map[string]string)
	if !haveTags {
		tagsIface, haveTagsIface = props["tags"].(map[string]any)
	}

	if name == "" {
		if haveTags {
			name, _ = tagsStr["name"]
		} else if haveTagsIface {
			name, _ = tagsIface["name"].(string)
		}
	}

	if name != "" {
		props["name"] = name
	}

	idIsValid := func(id any) (int64, bool) {
		switch v := props["id"].(type) {
		case string:
			idInt, err := strconv.ParseInt(v, 10, 64)
			return idInt, err == nil
		case int:
			return int64(v), true
		case uint:
			return int64(v), true
		case int64:
			return v, true
		case uint64:
			return int64(v), true
		}
		return 0, false
	}

	id, validId := idIsValid(props["id"])

	// drop meta and relations because they are object
	// and array respectively and seem empty.
	delete(props, "meta")
	delete(props, "relations")
	delete(props, "tags")

	if haveTags {
		if !validId {
			id, validId = idIsValid(props["id"])
		}
		for k, v := range tagsStr {
			if _, ok := props[k]; ok {
				continue
			}
			props[k] = v
		}
	}

	if haveTagsIface {
		if !validId {
			id, validId = idIsValid(props["id"])
		}
		for k, v := range tagsIface {
			if _, ok := props[k]; ok {
				continue
			}
			props[k] = v
		}
	}

	if validId {
		props["id"] = id
	}

	return
}
