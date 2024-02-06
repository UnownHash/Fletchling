package processor

import (
	"errors"

	"github.com/sirupsen/logrus"

	"github.com/UnownHash/Fletchling/geo"
	"github.com/UnownHash/Fletchling/processor/models"
)

type NestMatcher struct {
	logger      *logrus.Logger
	nestsRtree  *geo.FenceRTree[*models.Nest]
	spawnpoints map[uint64][]*models.Nest
	nests       map[int64]struct{}
}

// GetMatchingNests returns nests that contain the given spawnpointId. There is no locking.
// Returns nil if no nests match.
func (matcher *NestMatcher) GetMatchingNests(spawnpointId uint64, lat, lon float64) []*models.Nest {
	var nests []*models.Nest

	if matcher.spawnpoints == nil || spawnpointId == 0 {
		nests = matcher.nestsRtree.GetMatches(lat, lon)
	} else {
		nests = matcher.spawnpoints[spawnpointId]
	}
	if nests == nil {
		return nil
	}
	return nests
}

// AddNest stores a nest and its spawnpoints. There is no locking. There is no
// deduping.
func (matcher *NestMatcher) AddNest(nest *models.Nest, spawnpointIds []uint64) error {
	if _, ok := matcher.nests[nest.Id]; ok {
		return errors.New("nest exists")
	}

	err := matcher.nestsRtree.InsertGeometry(nest.Geometry.Geometry(), nest)
	if err != nil {
		return err
	}

	spawnpoints := matcher.spawnpoints

	for _, spawnpointId := range spawnpointIds {
		spawnpoints[spawnpointId] = append(
			spawnpoints[spawnpointId],
			nest,
		)
		if len(spawnpoints[spawnpointId]) > 1 {
			matcher.logger.Warnf("MATCHER[%s]: nests overlap: spawnpointId %d matches existing nest %s", nest, spawnpointId, spawnpoints[spawnpointId][0])
		}
	}

	matcher.nests[nest.Id] = struct{}{}

	return nil
}

func NewNestMatcher(logger *logrus.Logger, noSpawnpoints bool) *NestMatcher {
	matcher := &NestMatcher{
		logger:     logger,
		nestsRtree: geo.NewFenceRTree[*models.Nest](),
		nests:      make(map[int64]struct{}),
	}

	if !noSpawnpoints {
		matcher.spawnpoints = make(map[uint64][]*models.Nest)
	}

	return matcher
}
