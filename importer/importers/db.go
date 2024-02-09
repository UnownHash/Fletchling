package importers

import (
	"context"
	"encoding/json"
	"time"

	"github.com/paulmach/orb/geo"
	"github.com/paulmach/orb/geojson"
	"github.com/paulmach/orb/planar"
	"github.com/sirupsen/logrus"
	"gopkg.in/guregu/null.v4"

	"github.com/UnownHash/Fletchling/db_store"
	np_geo "github.com/UnownHash/Fletchling/geo"
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
		name, areaName, nestId, err := np_geo.NameAndIntIdFromFeature(feature)
		if err != nil {
			importer.logger.Warnf("DBImporter: skipping feature: %v", err)
			continue
		}

		fullName := name
		if areaName.Valid {
			fullName = areaName.String + "/" + name
		}

		geometry := feature.Geometry
		area := geo.Area(geometry)

		existingNest, _ := importer.nestsDBStore.GetNestByID(ctx, nestId)

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

		var updated null.Int
		var discarded null.String

		active := null.BoolFrom(true)

		if existingNest != nil {
			// don't wipe out db area_name
			if areaName.ValueOrZero() == "" {
				areaName = existingNest.AreaName
			}
			updated = existingNest.Updated
			active = existingNest.Active
			if active.ValueOrZero() {
				discarded = existingNest.Discarded
			}
		}

		if updated.ValueOrZero() == 0 {
			updated = null.IntFrom(nowEpoch)
		}

		center, _ := planar.CentroidArea(geometry)

		nest := &db_store.Nest{
			NestId:    nestId,
			Lat:       center.Lat(),
			Lon:       center.Lon(),
			Name:      name,
			Polygon:   polygon,
			AreaName:  areaName,
			M2:        null.FloatFrom(area),
			Active:    active,
			Discarded: discarded,
			Updated:   updated,
		}

		if existingNest != nil {
			// preserve the name, in case unknown names have been corrected.
			nest.Name = existingNest.Name
			nest.Spawnpoints = existingNest.Spawnpoints
			nest.PokemonId = existingNest.PokemonId
			nest.PokemonForm = existingNest.PokemonForm
			nest.PokemonAvg = existingNest.PokemonAvg
			nest.PokemonRatio = existingNest.PokemonRatio
			nest.PokemonCount = existingNest.PokemonCount
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
