package importers

import (
	"context"
	"errors"
	"fmt"
	"github.com/UnownHash/Fletchling/util"
	"strconv"

	"github.com/paulmach/orb/geojson"
	"github.com/sirupsen/logrus"

	np_geo "github.com/UnownHash/Fletchling/geo"
	"github.com/UnownHash/Fletchling/koji_client"
)

type KojiImporter struct {
	logger           *logrus.Logger
	kojiCli          *koji_client.AdminClient
	projectName      string
	createProperties bool
}

func (*KojiImporter) ImporterName() string {
	return "koji"
}

func looksLikeNumber(s string) bool {
	for _, r := range s {
		// throwing in .-+ to be extra safe.
		if (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '+' {
			continue
		}
		return false
	}
	return true
}

func (importer *KojiImporter) importFeature(ctx context.Context, feature *geojson.Feature, projects []int, geofencesByName map[string]*koji_client.Geofence, geofencesByNestId map[int64]*koji_client.Geofence) *koji_client.Geofence {
	name, areaName, nestId, err := np_geo.NameAndIntIdFromFeature(feature)
	if err != nil {
		importer.logger.Warnf("KojiImporter: skipping feature: %v", err)
		return nil
	}

	// https://github.com/TurtIeSocks/Koji/issues/220
	if looksLikeNumber(name) {
		name = "Nest " + name
	}

	if geofence := geofencesByNestId[nestId]; geofence != nil {
		importer.logger.Warnf("KojiImporter: skipping feature '%s': id from properties(nestId -> %d) exists with name '%s'", name, nestId, geofence.Name)
		return nil
	}

	var origGeofenceId int

	// names must be unique in koji. submitting duplicate name will actually update current entry.
	checkDuplicate := func(name string) string {
		geofence := geofencesByName[name]
		if geofence == nil {
			return name
		}

		featureCenter := util.GetPolygonLabelPoint(feature.Geometry)

		altName := name + fmt.Sprintf(" at %0.5f,%0.5f", featureCenter.Lat(), featureCenter.Lon())

		geofenceAlt := geofencesByName[altName]
		if geofenceAlt == nil {
			importer.logger.Warnf(
				"KojiImporter: using name '%s' for original feature name '%s': original name exists",
				altName,
				name,
			)
			return altName
		}

		origGeofenceId = geofenceAlt.Id
		importer.logger.Warnf(
			"KojiImporter: using name '%s' for original feature name '%s': both names exist (will update)",
			altName,
			name,
		)

		return altName
	}

	name = checkDuplicate(name)
	if name == "" {
		return nil
	}

	feature.Properties["name"] = name

	var parentId *int

	if parent, _ := feature.Properties["parent"].(string); parent != "" {
		parentGeofence := geofencesByName[parent]
		if parentGeofence == nil {
			importer.logger.Warnf("KojiImporter: parent '%s' exists in properties but no such geofence exists. Importing anyway...", parent)
		} else {
			parentId = &parentGeofence.Id
		}
	}

	fullName := name
	if areaName.Valid {
		fullName = areaName.String + "/" + name
	}

	geometry := feature.Geometry

	properties := make([]koji_client.GeofenceProperty, len(feature.Properties))
	idx := 0

	for k, v := range feature.Properties {
		var prop *koji_client.Property
		var err error

		if !importer.createProperties || v == nil {
			prop, err = importer.kojiCli.GetPropertyByName(k)
		} else {
			prop, err = importer.kojiCli.GetOrCreateProperty(k, v)
		}
		if err != nil {
			importer.logger.Warnf(
				"KojiImporter: skipping property '%s' for feature '%s': %v",
				k,
				fullName,
				err,
			)
			continue
		}
		if prop == nil {
			continue
		}
		properties[idx] = koji_client.GeofenceProperty{
			PropertyId: prop.PropertyId,
			Name:       k,
			Value:      v,
		}
		idx++
	}

	properties = properties[:idx]
	jsonGeometry := geojson.NewGeometry(geometry)

	geofence, err := importer.kojiCli.CreateGeofence(
		&koji_client.Geofence{
			Name:       name,
			GeoType:    jsonGeometry.Type,
			Mode:       "unset",
			Parent:     parentId,
			Geometry:   jsonGeometry,
			Projects:   projects,
			Properties: properties,
		},
	)

	if err != nil {
		importer.logger.Infof("KojiImporter: skipping feature '%s': Failed to create geofence: %v", fullName, err)
		return nil
	}

	geofencesByName[geofence.Name] = geofence
	geofencesByNestId[nestId] = geofence

	if geofence.Id == origGeofenceId {
		importer.logger.Infof("KojiImporter: Updated geofence %s(%d)(nestId %d)", geofence.Name, geofence.Id, nestId)
	} else {
		importer.logger.Infof("KojiImporter: Created geofence %s(%d)(nestId %d)", geofence.Name, geofence.Id, nestId)
	}

	return geofence
}

func (importer *KojiImporter) ImportFeatures(ctx context.Context, features []*geojson.Feature) error {
	importer.kojiCli.RefreshProperties()
	project, err := importer.kojiCli.GetProjectByName(importer.projectName)
	if err != nil {
		return err
	}

	projectGeofences := make(map[int]struct{})
	for _, geofenceId := range project.Geofences {
		projectGeofences[geofenceId] = struct{}{}
	}

	geofencesByName := make(map[string]*koji_client.Geofence)
	geofencesByNestId := make(map[int64]*koji_client.Geofence)

	geofences, err := importer.kojiCli.GetAllGeofencesFull()
	if err != nil {
		return err
	}

	for _, geofence := range geofences {
		geofencesByName[geofence.Name] = geofence
		if _, ok := projectGeofences[geofence.Id]; ok {
			nestId, err := nestIdFromProperties(geofence.Properties)
			if err != nil {
				continue
			}
			geofencesByNestId[nestId] = geofence
		}
	}

	projectIds := []int{project.Id}
	for _, feature := range features {
		importer.importFeature(ctx, feature, projectIds, geofencesByName, geofencesByNestId)
	}

	return nil
}

func NewKojiImporter(logger *logrus.Logger, kojiClient *koji_client.AdminClient, projectName string, createProperties bool) (*KojiImporter, error) {
	importer := &KojiImporter{
		logger:           logger,
		kojiCli:          kojiClient,
		projectName:      projectName,
		createProperties: createProperties,
	}
	return importer, nil
}

func nestIdFromProperties(props []koji_client.GeofenceProperty) (int64, error) {
	for _, prop := range props {
		if prop.Name == "id" {
			switch v := prop.Value.(type) {
			case string:
				nestId, err := strconv.ParseInt(v, 10, 64)
				if err != nil {
					return 0, fmt.Errorf("id '%v' can't be parsed as int", v)
				}
				return nestId, nil
			case int:
				return int64(v), nil
			case uint:
				return int64(v), nil
			case int64:
				return v, nil
			case uint64:
				return int64(v), nil
			default:
				return 0, fmt.Errorf("id '%v' type '%T' not supported", v, v)
			}
		}
	}
	return 0, errors.New("no id (nest id) found in properties")
}
