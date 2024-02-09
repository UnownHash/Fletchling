package db_store

import "github.com/sirupsen/logrus"

type DBStore struct {
	*NestsDBStore
	*GolbatDBStore
}

func NewDBStore(nestsConfig, golbatConfig DBConfig, logger *logrus.Logger) (*DBStore, error) {
	nestsDBStore, err := NewNestsDBStore(nestsConfig, logger)
	if err != nil {
		return nil, err
	}
	golbatDBStore, err := NewGolbatDBStore(golbatConfig, logger)
	if err != nil {
		return nil, err
	}
	return &DBStore{nestsDBStore, golbatDBStore}, nil
}
