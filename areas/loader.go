package areas

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/UnownHash/Fletchling/geo"
	"github.com/UnownHash/Fletchling/koji_client"
	"github.com/paulmach/orb/geojson"
	"github.com/sirupsen/logrus"
)

type AreasCache struct {
	logger   *logrus.Logger
	areas    []*geojson.Feature
	areasMap map[string]*geojson.Feature
}

func (cache *AreasCache) SetAreas(areas []*geojson.Feature) *AreasCache {
	areasMap := make(map[string]*geojson.Feature)
	for _, area := range areas {
		name, _ := area.Properties["name"].(string)
		if name == "" {
			cache.logger.Warn("skipping area with empty name. make sure name property is set.")
			continue
		}
		areasMap[name] = area
	}
	cache.areas = areas
	cache.areasMap = areasMap
	return cache
}

func (cache *AreasCache) Len() int {
	return len(cache.areas)
}

func (cache *AreasCache) GetAllAreas() []*geojson.Feature {
	return cache.areas[:]
}

func (cache *AreasCache) GetArea(areaName string) *geojson.Feature {
	return cache.areasMap[areaName]
}

type AreasLoader struct {
	logger        *logrus.Logger
	kojiClient    *koji_client.APIClient
	kojiProject   string
	filename      string
	cacheDir      string
	cacheFilename string
	areasCache    atomic.Pointer[AreasCache]
}

func (loader *AreasLoader) FullCachePath() string {
	return filepath.Join(loader.cacheDir, loader.cacheFilename)
}

func (loader *AreasLoader) updateCache(areas []*geojson.Feature) error {
	if loader.cacheFilename == "" {
		return nil
	}

	f, err := os.CreateTemp(loader.cacheDir, loader.cacheFilename+".*")
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	err = encoder.Encode(areas)
	if err != nil {
		unlinkErr := os.Remove(f.Name())
		if unlinkErr != nil {
			loader.logger.Warnf("failed to remove tmpfile '%s': %v", f.Name(), unlinkErr)
		}
		return err
	}

	err = os.Rename(f.Name(), loader.FullCachePath())
	if err != nil {
		return fmt.Errorf("failed to rename tmp cache file: %s -> %s: %v", f.Name(), loader.cacheFilename, err)
	}

	return nil
}

func (loader *AreasLoader) loadFromCache() error {
	if loader.cacheFilename == "" {
		return nil
	}

	f, err := os.Open(loader.FullCachePath())
	if err != nil {
		return err
	}
	defer f.Close()

	var areas []*geojson.Feature

	decoder := json.NewDecoder(f)
	if err := decoder.Decode(&areas); err != nil {
		return err
	}

	loader.areasCache.Store((&AreasCache{logger: loader.logger}).SetAreas(areas))

	return nil
}

func (loader *AreasLoader) getAreasCache(ctx context.Context) *AreasCache {
	areasCache := loader.areasCache.Load()
	if areasCache == nil || areasCache.Len() == 0 {
		loader.ReloadAreas(ctx)
		areasCache = loader.areasCache.Load()
	}
	return areasCache
}

func (loader *AreasLoader) GetAllAreas(ctx context.Context) []*geojson.Feature {
	areasCache := loader.getAreasCache(ctx)
	if areasCache == nil {
		return nil
	}
	return areasCache.GetAllAreas()
}

func (loader *AreasLoader) GetArea(ctx context.Context, areaName string) *geojson.Feature {
	areasCache := loader.getAreasCache(ctx)
	if areasCache == nil {
		return nil
	}
	return areasCache.GetArea(areaName)
}

func (loader *AreasLoader) ReloadAreas(ctx context.Context) (err error) {
	var areas []*geojson.Feature

	if filename := loader.filename; filename == "" {
		loader.logger.Infof("Reloading areas from koji project '%s'", loader.kojiProject)

		var fc *geojson.FeatureCollection
		fc, err = loader.kojiClient.GetFeatureCollection(ctx, loader.kojiProject)
		if err != nil {
			return
		}
		areas = fc.Features
		loader.logger.Infof("Loaded %d area(s) from koji", len(areas))
	} else {
		loader.logger.Infof("Reloading areas from file '%s'", filename)
		areas, err = geo.LoadFeaturesFromFile(filename)
		if err != nil {
			return
		}
		loader.logger.Infof("Loaded %d area(s) from file '%s'", len(areas), filename)
	}

	loader.areasCache.Store((&AreasCache{logger: loader.logger}).SetAreas(areas))
	if cacheErr := loader.updateCache(areas); cacheErr == nil {
		loader.logger.Info("Updated areas cache file")
	} else {
		loader.logger.Warnf("Failed to update areas cache file: %v", cacheErr)
	}

	return
}

func NewAreasLoader(logger *logrus.Logger, config Config) (*AreasLoader, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	loader := &AreasLoader{
		logger: logger,
	}

	var err error

	if config.Filename == "" {
		loader.kojiProject = config.KojiProject
		loader.kojiClient, err = koji_client.NewAPIClient(
			logger,
			config.KojiBaseUrl,
			config.KojiToken,
		)
	} else {
		loader.filename = config.Filename
	}

	if err != nil {
		return nil, fmt.Errorf("AreasLoader: failed to create koji client: %w", err)
	}

	return loader, nil
}
