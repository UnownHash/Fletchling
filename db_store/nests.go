package db_store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	migrate_mysql "github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jmoiron/sqlx"
	"github.com/paulmach/orb/geojson"
	"github.com/sirupsen/logrus"
	"gopkg.in/guregu/null.v4"
)

type NestsDBStore struct {
	logger *logrus.Logger
	db     *sqlx.DB
}

type NestPartialUpdate struct {
	AreaName     *null.String
	Spawnpoints  *int64
	M2           *float64
	Active       *bool
	PokemonId    *null.Int
	PokemonForm  *null.Int
	PokemonAvg   *null.Float
	PokemonRatio *null.Float
	PokemonCount *null.Float
	Discarded    *null.String
	Updated      *null.Int
}

type Nest struct {
	NestId       int64       `db:"nest_id"`
	Lat          float64     `db:"lat"`
	Lon          float64     `db:"lon"`
	Name         string      `db:"name"`
	Polygon      []byte      `db:"polygon"`
	AreaName     null.String `db:"area_name"`
	Spawnpoints  null.Int    `db:"spawnpoints"`
	M2           null.Float  `db:"m2"`
	Active       null.Bool   `db:"active"`
	PokemonId    null.Int    `db:"pokemon_id"`
	PokemonForm  null.Int    `db:"pokemon_form"`
	PokemonAvg   null.Float  `db:"pokemon_avg"`
	PokemonRatio null.Float  `db:"pokemon_ratio"`
	PokemonCount null.Float  `db:"pokemon_count"`
	Discarded    null.String `db:"discarded"`
	Updated      null.Int    `db:"updated"`
}

func (nest *Nest) Geometry() (*geojson.Geometry, error) {
	var geom geojson.Geometry

	err := json.Unmarshal(nest.Polygon, &geom)
	if err != nil {
		return nil, err
	}
	return &geom, nil
}

func (nest *Nest) UpdatedTime() time.Time {
	return time.Unix(nest.Updated.ValueOrZero(), 0)
}

const (
	nestColumns       = "nest_id,lat,lon,name,polygon,area_name,spawnpoints,m2,active,pokemon_id,pokemon_form,pokemon_avg,pokemon_ratio,pokemon_count,discarded,updated"
	nestSelectColumns = "nest_id,lat,lon,name,ST_AsGeoJSON(polygon) as polygon,area_name,spawnpoints,m2,active,pokemon_id,pokemon_form,pokemon_avg,pokemon_ratio,pokemon_count,discarded,updated"
	nestColumnsNoPoly = "nest_id,lat,lon,name,area_name,spawnpoints,m2,active,pokemon_id,pokemon_form,pokemon_avg,pokemon_ratio,pokemon_count,discarded,updated"
)

// InsertOrUpdateNest will insert a new nest or update an existing one. If updating,
// the nesting pokemon and info will be preserved. This is meant for importing into the
// DB.
func (st *NestsDBStore) InsertOrUpdateNest(ctx context.Context, nest *Nest) error {
	const nestBaseInsertQuery = "INSERT into nests (" + nestColumns + ") VALUES (:nest_id,:lat,:lon,:name,ST_GeomFromGeoJSON(:polygon),:area_name,:spawnpoints,:m2,:active,:pokemon_id,:pokemon_form,:pokemon_avg,:pokemon_ratio,:pokemon_count,:discarded,:updated)"
	const nestInsertUpdateQuery = nestBaseInsertQuery + " ON DUPLICATE KEY UPDATE name=VALUES(name),lat=VALUES(lat),lon=VALUES(lon),polygon=VALUES(polygon),area_name=VALUES(area_name),spawnpoints=VALUES(spawnpoints),m2=VALUES(m2),active=VALUES(active),discarded=VALUES(discarded),updated=VALUES(updated)"

	_, err := st.db.NamedExecContext(ctx, nestInsertUpdateQuery, nest)
	return err
}

func (st *NestsDBStore) GetNestById(ctx context.Context, nestId int64) (*Nest, error) {
	const query = "SELECT " + nestSelectColumns + " FROM nests WHERE nest_id=?"

	row := st.db.QueryRowxContext(ctx, query, nestId)

	var nest Nest

	if err := row.StructScan(&nest); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &nest, nil
}

func (st *NestsDBStore) GetAllNests(ctx context.Context) ([]*Nest, error) {
	const query = "SELECT " + nestSelectColumns + " FROM nests"

	rows, err := st.db.QueryxContext(ctx, query)
	if err != nil {
		if err == sql.ErrNoRows {
			err = nil
		}
		return nil, err
	}

	nests := make([]*Nest, 0, 64)

	for rows.Next() {
		var nest Nest

		if err := rows.StructScan(&nest); err != nil {
			if err == sql.ErrNoRows {
				return nil, nil
			}
			return nil, err
		}

		nests = append(nests, &nest)
	}

	return nests, nil
}

func (st *NestsDBStore) GetNestsWithoutPolygon(ctx context.Context, nestIds ...int64) (map[int64]*Nest, error) {
	const baseQuery = "SELECT " + nestColumnsNoPoly + " FROM nests WHERE "
	const batchSize = 500

	l := len(nestIds)
	if l == 0 {
		return nil, nil
	}

	var params [batchSize]any

	whereSz := batchSize
	if l < whereSz {
		whereSz = l
	}

	// Use OR and avoid https://jira.mariadb.org/browse/MDEV-26232
	query := baseQuery + "(" + strings.Repeat("nest_id=? OR ", whereSz-1) + "nest_id=?)"

	nests := make(map[int64]*Nest, len(nestIds))

	for l > 0 {
		thisLength := l
		if thisLength > batchSize {
			thisLength = batchSize
		}

		for i := 0; i < thisLength; i++ {
			params[i] = nestIds[i]
		}

		nestIds = nestIds[thisLength:]
		l -= thisLength

		if thisLength != whereSz {
			// Use OR and avoid https://jira.mariadb.org/browse/MDEV-26232
			query = baseQuery + "(" + strings.Repeat("nest_id=? OR ", thisLength-1) + "nest_id=?)"
		}

		rows, err := st.db.QueryxContext(ctx, query, params[:thisLength]...)
		if err != nil {
			return nil, err
		}

		for rows.Next() {
			var nest Nest

			if err := rows.StructScan(&nest); err != nil {
				if err == sql.ErrNoRows {
					// none exist in this batch.
					break
				}
				return nil, err
			}

			nests[nest.NestId] = &nest
		}
	}

	return nests, nil
}

func (st *NestsDBStore) GetInactiveNests(ctx context.Context) ([]*Nest, error) {
	const query = "SELECT " + nestSelectColumns + " FROM nests WHERE NOT active"
	rows, err := st.db.QueryxContext(ctx, query)
	if err != nil {
		if err == sql.ErrNoRows {
			err = nil
		}
		return nil, err
	}

	nests := make([]*Nest, 0, 64)

	for rows.Next() {
		var nest Nest

		if err := rows.StructScan(&nest); err != nil {
			if err == sql.ErrNoRows {
				return nil, nil
			}
			return nil, err
		}

		nests = append(nests, &nest)
	}

	return nests, nil
}

func (st *NestsDBStore) GetActiveNests(ctx context.Context) ([]*Nest, error) {
	const query = "SELECT " + nestSelectColumns + " FROM nests WHERE active"

	rows, err := st.db.QueryxContext(ctx, query)
	if err != nil {
		if err == sql.ErrNoRows {
			err = nil
		}
		return nil, err
	}

	nests := make([]*Nest, 0, 64)

	for rows.Next() {
		var nest Nest

		if err := rows.StructScan(&nest); err != nil {
			if err == sql.ErrNoRows {
				return nil, nil
			}
			return nil, err
		}

		nests = append(nests, &nest)
	}

	return nests, nil
}

func (st *NestsDBStore) UpdateNestPartial(ctx context.Context, nestId int64, nestUpdate *NestPartialUpdate) error {
	var args [12]any
	var query bytes.Buffer

	query.WriteString("UPDATE nests SET ")

	n := 0

	addValue := func(str string, v any) {
		if n > 0 {
			query.WriteRune(',')
		}
		query.WriteString(str)
		args[n] = v
		n++
	}

	if v := nestUpdate.Updated; v != nil {
		addValue("updated=?", *v)
	}
	if v := nestUpdate.AreaName; v != nil {
		addValue("area_name=?", *v)
	}
	if v := nestUpdate.Spawnpoints; v != nil {
		addValue("spawnpoints=?", *v)
	}
	if v := nestUpdate.M2; v != nil {
		addValue("m2=?", *v)
	}
	if v := nestUpdate.PokemonId; v != nil {
		addValue("pokemon_id=?", *v)
	}
	if v := nestUpdate.PokemonForm; v != nil {
		addValue("pokemon_form=?", *v)
	}
	if v := nestUpdate.PokemonAvg; v != nil {
		addValue("pokemon_avg=?", *v)
	}
	if v := nestUpdate.PokemonRatio; v != nil {
		addValue("pokemon_ratio=?", *v)
	}
	if v := nestUpdate.PokemonCount; v != nil {
		addValue("pokemon_count=?", *v)
	}
	if v := nestUpdate.Active; v != nil {
		addValue("active=?", *v)
	}
	if v := nestUpdate.Discarded; v != nil {
		addValue("discarded=?", *v)
	}

	if n == 0 {
		// nothing to update
		return nil
	}

	query.WriteString(" WHERE nest_id=?")
	args[n] = nestId
	n++

	_, err := st.db.ExecContext(ctx, query.String(), args[:n]...)
	return err
}

func (st *NestsDBStore) DisableOverlappingNests(ctx context.Context, percent float64) (int64, error) {
	const query = `
        UPDATE nests SET active=0,discarded='overlap' WHERE nest_id IN (
          SELECT b.nest_id
          FROM nests a, nests b
          WHERE a.active = 1 AND b.active = 1 AND a.m2 > b.m2 AND ST_Intersects(a.polygon, b.polygon) AND ST_Area(ST_Intersection(a.polygon,b.polygon)) / ST_Area(b.polygon) * 100 > ?
		)`

	res, err := st.db.ExecContext(ctx, query, percent)
	if err == nil {
		return res.RowsAffected()
	}
	return 0, err
}

func NewNestsDBStore(config DBConfig, logger *logrus.Logger, migratePath string) (*NestsDBStore, error) {
	db, err := sqlx.Connect("mysql", config.AsDSN())
	if err != nil {
		return nil, err
	}

	if config.MaxPool > 0 {
		db.SetMaxOpenConns(config.MaxPool)
	}

	if migratePath == "" {
		logger.Infof("skipping nests_db migrations: no path given")
	} else {
		logger.Infof("running nests_db migrations")
		migrateConfig := &migrate_mysql.Config{
			MigrationsTable: "nests_schema_migrations",
			DatabaseName:    config.Db,
		}

		dbDriver, err := migrate_mysql.WithInstance(db.DB, migrateConfig)
		if err != nil {
			return nil, err
		}

		if !strings.HasPrefix(migratePath, "file://") {
			migratePath = "file://" + migratePath
		}

		m, err := migrate.NewWithDatabaseInstance(migratePath, config.Db, dbDriver)
		if err != nil {
			return nil, fmt.Errorf("failed to run nests DB migration: %w", err)
		}

		err = m.Up()
		if err != nil && err != migrate.ErrNoChange {
			return nil, err
		}
	}

	return &NestsDBStore{
		logger: logger,
		db:     db,
	}, nil
}
