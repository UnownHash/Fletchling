package nest_loader

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/UnownHash/Fletchling/db_store"
	"github.com/UnownHash/Fletchling/koji_client"
	"github.com/UnownHash/Fletchling/processor/models"
)

type KojiNestLoader struct {
	logger       *logrus.Logger
	kojiCli      *koji_client.APIClient
	projectName  string
	nestsDBStore *db_store.NestsDBStore
}

func (*KojiNestLoader) LoaderName() string {
	return "koji"
}

func (loader *KojiNestLoader) LoadNests(ctx context.Context) ([]*models.Nest, error) {
	fc, err := loader.kojiCli.GetFeatureCollection(loader.projectName)
	if err != nil {
		return nil, err
	}

	kojiNests := make([]*models.Nest, len(fc.Features))
	nestIds := make([]int64, len(fc.Features))

	idx := 0
	for _, feature := range fc.Features {
		nest, err := models.NewNestFromKojiFeature(feature)
		if err != nil {
			loader.logger.Warnf("NEST-LOAD[]: skipping geofence from koji: %v", err)
			continue
		}
		kojiNests[idx] = nest
		nestIds[idx] = nest.Id
		idx++
	}
	kojiNests = kojiNests[:idx]
	nestIds = nestIds[:idx]

	// resolve against what's in the DB so far.
	dbNests, err := loader.nestsDBStore.GetNestsWithoutPolygon(ctx, nestIds...)
	if err != nil {
		return nil, fmt.Errorf("failed to get existing nests from DB: %w", err)
	}

	for _, kojiNest := range kojiNests {
		dbNest := dbNests[kojiNest.Id]
		if dbNest == nil {
			dbNest = kojiNest.AsDBStoreNest()
		} else {
			kojiNest.ExistsInDb = true
			kojiNest.Spawnpoints = dbNest.Spawnpoints.ValueOrZero()
			kojiNest.AreaM2 = dbNest.M2.ValueOrZero()
			kojiNest.Active = dbNest.Active.ValueOrZero()
			kojiNest.Discarded = dbNest.Discarded.ValueOrZero()
			kojiNest.SetNestingPokemon(
				models.NestingPokemonInfoFromDBStore(dbNest),
			)
		}
	}
	return kojiNests, nil
}

func NewKojiNestLoader(logger *logrus.Logger, kojiCli *koji_client.APIClient, projectName string, nestsDBStore *db_store.NestsDBStore) *KojiNestLoader {
	st := &KojiNestLoader{
		logger:       logger,
		kojiCli:      kojiCli,
		projectName:  projectName,
		nestsDBStore: nestsDBStore,
	}
	return st
}
