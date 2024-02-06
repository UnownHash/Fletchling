package geo

import (
	"errors"
	"fmt"

	"github.com/paulmach/orb/geojson"
)

type AreaName struct {
	Parent   string
	Name     string
	FullName string
}

func (an AreaName) String() string {
	return an.FullName
}

func NewAreaName(parent, name string) AreaName {
	var fullName string

	if parent != "" {
		if name == "" || parent == name {
			fullName = parent
		} else {
			fullName = parent + "/" + name
		}
	} else if name != "" {
		fullName = name
	} else {
		fullName = "world"
	}

	return AreaName{
		Parent:   parent,
		Name:     name,
		FullName: fullName,
	}
}

type AreaMatcher struct {
	areas FenceRTree[AreaName]
}

func (matcher *AreaMatcher) GetMatchingAreas(lat, lon float64) []AreaName {
	return matcher.areas.GetMatches(lat, lon)
}

func (matcher *AreaMatcher) LoadFeatureCollection(featureCollection *geojson.FeatureCollection) (err error) {
	defer func() {
		r := recover()
		if err == nil {
			err = fmt.Errorf("panic during feature insert: %#v", r)
		}
		return
	}()

	for _, feature := range featureCollection.Features {
		parent := feature.Properties.MustString("parent", "")
		name := feature.Properties.MustString("name", "")
		if name == "" {
			err = errors.New("feature is missing a name")
			return
		}
		err = matcher.areas.InsertFeature(feature, NewAreaName(parent, name))
		if err != nil {
			return
		}
	}
	return
}
