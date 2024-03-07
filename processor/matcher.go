package processor

import (
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/UnownHash/Fletchling/geo"
	"github.com/UnownHash/Fletchling/processor/models"
)

type NestMatcher struct {
	logger     *logrus.Logger
	nestsRtree *geo.FenceRTree[*models.Nest]
	nests      map[int64]*models.Nest
}

// GetMatchingNests returns nests that contain the given lat, lon. There is no locking.
// Returns nil if no nests match.
func (matcher *NestMatcher) GetMatchingNests(lat, lon float64) []*models.Nest {
	return matcher.nestsRtree.GetMatches(lat, lon)
}

// AddNest stores a nest for later matching by lat/lon. There is no locking. If a
// nest exists already with the same Id, an error will be returned.
func (matcher *NestMatcher) AddNest(nest *models.Nest) error {
	if _, ok := matcher.nests[nest.Id]; ok {
		return fmt.Errorf("nest with id '%d' already exists", nest.Id)
	}

	err := matcher.nestsRtree.InsertGeometry(nest.Geometry.Geometry(), nest)
	if err != nil {
		return err
	}

	matcher.nests[nest.Id] = nest

	return nil
}

func (matcher *NestMatcher) Len() int {
	return len(matcher.nests)
}

// Returns a nest by its Id. There is no locking. Returns nil if nest is unknown.
func (matcher *NestMatcher) GetNestById(nestId int64) *models.Nest {
	return matcher.nests[nestId]
}

func (matcher *NestMatcher) GetAllNests() []*models.Nest {
	// nests don't change once loaded into this object, so no locking.
	nests := make([]*models.Nest, len(matcher.nests))
	idx := 0
	for _, nest := range matcher.nests {
		nests[idx] = nest
		idx++
	}
	return nests
}

func NewNestMatcher(logger *logrus.Logger) *NestMatcher {
	matcher := &NestMatcher{
		logger:     logger,
		nestsRtree: geo.NewFenceRTree[*models.Nest](),
		nests:      make(map[int64]*models.Nest),
	}
	return matcher
}
