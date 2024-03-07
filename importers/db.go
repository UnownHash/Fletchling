package importers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	orb_geo "github.com/paulmach/orb/geo"
	"github.com/paulmach/orb/geojson"
	"github.com/sirupsen/logrus"
	"gopkg.in/guregu/null.v4"

	"github.com/UnownHash/Fletchling/db_store"
	"github.com/UnownHash/Fletchling/geo"
)

type DBImporter struct {
	logger       *logrus.Logger
	nestsDBStore *db_store.NestsDBStore
}

func (*DBImporter) ImporterName() string {
	return "db"
}

func (importer *DBImporter) ImportFeatures(ctx context.Context, features []*geojson.Feature) error {
	nowEpoch := time.Now().Unix()

	for _, feature := range features {
		name, areaName, nestId, err := geo.NameAndIntIdFromFeature(feature)
		if err != nil {
			importer.logger.Warnf("DBImporter: skipping feature: %v", err)
			continue
		}

		fullName := fmt.Sprintf("%s(NestId %d)", name, nestId)
		if areaName.Valid {
			fullName = areaName.String + "/" + fullName
		}

		geometry := feature.Geometry
		area := orb_geo.Area(geometry)

		existingNest, _ := importer.nestsDBStore.GetNestById(ctx, nestId)

		jsonGeometry := geojson.NewGeometry(geometry)

		polygon, err := json.Marshal(jsonGeometry)
		if err != nil {
			importer.logger.Warnf(
				"DBImporter: skipping feature '%s': failed to marshal to geojson.Geometry: %v",
				fullName,
				err,
			)
			continue
		}

		center := geo.GetPolygonLabelPoint(geometry)
		nest := &db_store.Nest{
			NestId:  nestId,
			Lat:     center.Lat(),
			Lon:     center.Lon(),
			Name:    name,
			Polygon: polygon,
			M2:      null.FloatFrom(area),
		}

		if existingNest != nil {
			nest.Active = existingNest.Active
			nest.Updated = existingNest.Updated
			nest.Discarded = existingNest.Discarded

			// preserve the name, in case unknown names have been corrected.
			nest.Name = existingNest.Name
			nest.Spawnpoints = existingNest.Spawnpoints

			if nest.Active.ValueOrZero() {
				nest.PokemonId = existingNest.PokemonId
				nest.PokemonForm = existingNest.PokemonForm
				nest.PokemonAvg = existingNest.PokemonAvg
				nest.PokemonRatio = existingNest.PokemonRatio
				nest.PokemonCount = existingNest.PokemonCount
			}

			// prefer new areaName over DB
			if areaName.ValueOrZero() == "" {
				areaName = existingNest.AreaName
			}
		}

		nest.AreaName = areaName
		if nest.Active.ValueOrZero() {
			nest.Discarded.Valid = false
		} else {
			// ensure !NULL
			nest.Active = null.BoolFrom(false)
			if !nest.Discarded.Valid {
				nest.Discarded = null.StringFrom("unverified")
			}
		}

		if nest.Updated.ValueOrZero() == 0 {
			nest.Updated = null.IntFrom(nowEpoch)
		}

		err = importer.nestsDBStore.InsertOrUpdateNest(ctx, nest)
		if err != nil {
			importer.logger.Warnf(
				"DBImporter: skipping feature '%s': failed to insert/update DB: %v",
				fullName,
				err,
			)
			continue
		}
		if existingNest == nil {
			importer.logger.Infof("DBImporter: imported new nest '%s'", fullName)
		} else {
			importer.logger.Infof("DBImporter: updated existing nest '%s'", fullName)
		}
	}

	return nil
}

func NewDBImporter(logger *logrus.Logger, nestsDBStore *db_store.NestsDBStore) (*DBImporter, error) {
	importer := &DBImporter{
		logger:       logger,
		nestsDBStore: nestsDBStore,
	}
	return importer, nil
}
