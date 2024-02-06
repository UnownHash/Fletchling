package nest_loader

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/UnownHash/Fletchling/db_store"
	"github.com/UnownHash/Fletchling/processor/models"
)

type DBNestLoader struct {
	logger  *logrus.Logger
	dbStore *db_store.NestsDBStore
}

func (*DBNestLoader) LoaderName() string {
	return "db"
}

func (loader *DBNestLoader) LoadNests(ctx context.Context) ([]*models.Nest, error) {
	dbNests, err := loader.dbStore.GetAllNests(ctx)
	if err != nil {
		return nil, err
	}
	nests := make([]*models.Nest, len(dbNests))
	idx := 0
	for _, dbNest := range dbNests {
		nest, err := models.NewNestFromDBStore(dbNest)
		if err != nil {
			loader.logger.Warnf("skipping nest %d/%s: %s", dbNest.NestId, dbNest.Name, err)
			continue
		}
		nests[idx] = nest
		idx++
	}
	nests = nests[:idx]
	return nests, nil
}

func NewDBNestLoader(logger *logrus.Logger, dbStore *db_store.NestsDBStore) *DBNestLoader {
	return &DBNestLoader{
		logger:  logger,
		dbStore: dbStore,
	}
}
