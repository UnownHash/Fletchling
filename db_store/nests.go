package db_store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	migrate_mysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jmoiron/sqlx"
	"github.com/paulmach/orb/geojson"
	"github.com/sirupsen/logrus"
	"gopkg.in/guregu/null.v4"
)

type dbQueryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	NamedExecContext(context.Context, string, any) (sql.Result, error)
}

type NestPartialUpdate struct {
	AreaName     *null.String
	Spawnpoints  *null.Int
	M2           *null.Float
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

type NestsDBStore struct {
	logger *logrus.Logger
	db     *sqlx.DB
	dbName string
	dsn    string
}

func (st *NestsDBStore) updateNestPartial(ctx context.Context, queryer dbQueryer, nestId int64, nestUpdate *NestPartialUpdate) error {
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

	if st.logger.Level >= logrus.DebugLevel {
		st.logger.Debugf("Running partial nest DB update: %s, %#v", query.String(), args[:n])
	}

	_, err := queryer.ExecContext(ctx, query.String(), args[:n]...)
	return err
}

func (st *NestsDBStore) disableOverlappingNests(ctx context.Context, queryer dbQueryer, percent float64) (int64, error) {
	const query = `CALL fl_nest_filter_overlap(?)`
	res, err := queryer.ExecContext(ctx, query, percent)
	if err == nil {
		return res.RowsAffected()
	}

	return 0, err
}

func (nest *Nest) FullName() string {
	var namePrefix string
	if an := nest.AreaName.ValueOrZero(); an != "" {
		namePrefix = nest.AreaName.String + "/"
	}
	return fmt.Sprintf("%s%s(NestId:%d)", namePrefix, nest.Name, nest.NestId)
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
	nestColumns             = "nest_id,lat,lon,name,polygon,area_name,spawnpoints,m2,active,pokemon_id,pokemon_form,pokemon_avg,pokemon_ratio,pokemon_count,discarded,updated"
	nestSelectColumns       = "nest_id,lat,lon,name,ST_AsGeoJSON(polygon) as polygon,area_name,spawnpoints,m2,active,pokemon_id,pokemon_form,pokemon_avg,pokemon_ratio,pokemon_count,discarded,updated"
	nestSelectColumnsNoPoly = "nest_id,lat,lon,name,area_name,spawnpoints,m2,active,pokemon_id,pokemon_form,pokemon_avg,pokemon_ratio,pokemon_count,discarded,updated"
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

func (st *NestsDBStore) GetAllNests(ctx context.Context) (nests []*Nest, err error) {
	const query = "SELECT " + nestSelectColumns + " FROM nests"

	rows, err := st.db.QueryxContext(ctx, query)
	if err != nil {
		return nil, err
	}

	defer func() { err = closeRows(rows, err) }()

	nests = make([]*Nest, 0, 64)

	for rows.Next() {
		var nest Nest

		if err = rows.StructScan(&nest); err != nil {
			return
		}

		nests = append(nests, &nest)
	}

	return nests, nil
}

func (st *NestsDBStore) iterateNestsBatch(ctx context.Context, fn func(Nest) error, qry string, args ...any) (numRows uint64, lastId int64, err error) {
	rows, err := st.db.QueryxContext(ctx, qry, args...)
	if err != nil {
		return
	}

	defer func() { err = closeRows(rows, err) }()

	for rows.Next() {
		var nest Nest

		if err = rows.StructScan(&nest); err != nil {
			return
		}

		numRows++
		lastId = nest.NestId

		if err = fn(nest); err != nil {
			return
		}
	}

	err = rows.Err()

	return
}

type StreamNestsOpts struct {
	IncludePolygon bool
}

func (st *NestsDBStore) StreamNests(ctx context.Context, opts StreamNestsOpts, ch chan<- Nest) error {
	const queryPrefixPolygon = "SELECT " + nestSelectColumns + " FROM nests "
	const queryPrefixNoPolygon = "SELECT " + nestSelectColumnsNoPoly + " FROM nests "
	const querySuffix = "ORDER BY nest_id ASC LIMIT 1000"

	var queryPrefix string

	if opts.IncludePolygon {
		queryPrefix = queryPrefixPolygon
	} else {
		queryPrefix = queryPrefixNoPolygon
	}

	defer close(ch)

	fn := func(nest Nest) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ch <- nest:
			return nil
		}
	}

	numRows, lastId, err := st.iterateNestsBatch(ctx, fn, queryPrefix+querySuffix)
	if err != nil {
		return err
	}

	if numRows < 1000 {
		return nil
	}

	for {
		numRows, lastId, err = st.iterateNestsBatch(
			ctx,
			fn,
			queryPrefix+"WHERE nest_id > ? "+querySuffix,
			lastId,
		)
		if err != nil {
			return err
		}
		if numRows < 1000 {
			break
		}
	}

	return nil
}

type IterateNestsConcurrentlyOpts struct {
	Concurrency    int
	IncludePolygon bool
}

func (st *NestsDBStore) IterateNestsConcurrently(ctx context.Context, opts IterateNestsConcurrentlyOpts, fn func(Nest) error) error {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 2
	}

	var wg sync.WaitGroup
	workCh := make(chan Nest, 64)

	ctx, cancelFn := context.WithCancel(ctx)
	defer func() {
		// cancelling context will cause StreamNests to
		// close workCh (before it returns) and all workers will
		// then exit. We ensure they are all shut down before
		// returning:
		cancelFn()
		wg.Wait()
	}()

	var failOnce sync.Once
	var loopFail error

	for _ = range opts.Concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					failOnce.Do(func() {
						loopFail = ctx.Err()
					})
					return
				case nest, ok := <-workCh:
					if !ok {
						// channel closed
						return
					}
					if err := fn(nest); err != nil {
						failOnce.Do(func() {
							loopFail = err
						})
						cancelFn()
						return
					}
				}
			}
		}()
	}

	err := st.StreamNests(
		ctx,
		StreamNestsOpts{
			IncludePolygon: opts.IncludePolygon,
		},
		workCh,
	)

	if err != nil {
		return err
	}

	wg.Wait()

	return loopFail
}

func (st *NestsDBStore) GetNestsWithoutPolygon(ctx context.Context, nestIds ...int64) (map[int64]*Nest, error) {
	const baseQuery = "SELECT " + nestSelectColumnsNoPoly + " FROM nests WHERE "
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

		_, _, err := st.iterateNestsBatch(
			ctx,
			func(nest Nest) error {
				nests[nest.NestId] = &nest
				return nil
			},
			query,
			params[:thisLength]...,
		)

		if err != nil {
			return nil, err
		}
	}

	return nests, nil
}

func (st *NestsDBStore) GetInactiveNests(ctx context.Context) (nests []*Nest, err error) {
	const query = "SELECT " + nestSelectColumns + " FROM nests WHERE NOT active"
	rows, err := st.db.QueryxContext(ctx, query)
	if err != nil {
		return
	}

	defer func() {
		err = closeRows(rows, err)
		if err != nil {
			nests = nil
		}
	}()

	for rows.Next() {
		var nest Nest

		if err = rows.StructScan(&nest); err != nil {
			return
		}

		nests = append(nests, &nest)
	}

	err = rows.Err()

	return
}

func (st *NestsDBStore) GetActiveNests(ctx context.Context) (nests []*Nest, err error) {
	const query = "SELECT " + nestSelectColumns + " FROM nests WHERE active"

	rows, err := st.db.QueryxContext(ctx, query)
	if err != nil {
		return
	}

	defer func() {
		err = closeRows(rows, err)
		if err != nil {
			nests = nil
		}
	}()

	nests = make([]*Nest, 0)

	for rows.Next() {
		var nest Nest

		if err = rows.StructScan(&nest); err != nil {
			return
		}

		nests = append(nests, &nest)
	}

	err = rows.Err()

	return
}

func (st *NestsDBStore) GetNestAreas(ctx context.Context) (areas []string, err error) {
	const query = "SELECT DISTINCT(area_name) FROM nests where area_name is NOT NULL"

	rows, err := st.db.QueryxContext(ctx, query)
	if err != nil {
		return
	}

	defer func() {
		err = closeRows(rows, err)
		if err != nil {
			areas = nil
		}
	}()

	areas = make([]string, 0)

	for rows.Next() {
		var s string

		if err = rows.Scan(&s); err != nil {
			return
		}

		areas = append(areas, s)
	}

	err = rows.Err()

	return
}

func (st *NestsDBStore) UpdateNestPartial(ctx context.Context, nestId int64, nestUpdate *NestPartialUpdate) error {
	return st.updateNestPartial(ctx, st.db, nestId, nestUpdate)
}

func (st *NestsDBStore) DisableOverlappingNests(ctx context.Context, percent float64) (int64, error) {
	return st.disableOverlappingNests(ctx, st.db, percent)
}

func (st *NestsDBStore) Migrate(migratePath string) error {
	st.logger.Infof("running nests_db migrations")

	migrateConfig := &migrate_mysql.Config{
		MigrationsTable: "nests_schema_migrations",
		DatabaseName:    st.dbName,
	}

	db, err := sql.Open("mysql", st.dsn+"?&multiStatements=true")
	if err != nil {
		return fmt.Errorf("failed to connect to the DB: %w", err)
	}

	dbDriver, err := migrate_mysql.WithInstance(db, migrateConfig)
	if err != nil {
		return err
	}

	if !strings.HasPrefix(migratePath, "file://") {
		migratePath = "file://" + migratePath
	}

	m, err := migrate.NewWithDatabaseInstance(migratePath, st.dbName, dbDriver)
	if err != nil {
		return fmt.Errorf("failed to run nests DB migration: %w", err)
	}

	err = m.Up()
	if err != nil && err != migrate.ErrNoChange {
		return err
	}

	return nil
}

func (st *NestsDBStore) CheckMigrate(migratePath string) (curVersion, maxVersion uint, err error) {
	migrateConfig := &migrate_mysql.Config{
		MigrationsTable: "nests_schema_migrations",
		DatabaseName:    st.dbName,
	}

	dbDriver, err := migrate_mysql.WithInstance(st.db.DB, migrateConfig)
	if err != nil {
		return 0, 0, err
	}

	if !strings.HasPrefix(migratePath, "file://") {
		migratePath = "file://" + migratePath
	}

	sourceDrv, err := source.Open(migratePath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get migrate source: %w", err)
	}

	maxVersion, err = sourceDrv.First()

	if err != nil {
		if err == os.ErrNotExist {
			err = fmt.Errorf("error finding first migration??")
		}
		err = fmt.Errorf("error iterating migrations: %w", err)
		return
	}

	for {
		var vers uint

		vers, err = sourceDrv.Next(maxVersion)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				break
			}
			err = fmt.Errorf("error iterating migrations: %w", err)
			return
		}
		maxVersion = vers
	}

	m, err := migrate.NewWithDatabaseInstance(migratePath, st.dbName, dbDriver)
	if err != nil {
		err = fmt.Errorf("failed to get migrate instance: %w", err)
		return
	}

	var dirty bool

	curVersion, dirty, err = m.Version()
	if err != nil {
		if err != migrate.ErrNilVersion {
			err = fmt.Errorf("failed to get current migration version: %w", err)
			return
		}
		curVersion = 0
		err = nil
	}

	if dirty {
		err = errors.New("migrations are in a failed state. try to fix/restart fletchling to re-run.")
		return
	}

	if curVersion != maxVersion {
		err = fmt.Errorf("nests db is at migration version %d, code is at %d, dirty:%t. restart fletching to update.", curVersion, maxVersion, dirty)
		return
	}

	return
}

func NewNestsDBStore(config DBConfig, logger *logrus.Logger) (*NestsDBStore, error) {
	dsn := config.AsDSN()

	db, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		return nil, err
	}

	if config.MaxPool <= 0 {
		config.MaxPool = 10
	}

	db.SetMaxOpenConns(config.MaxPool)
	db.SetMaxIdleConns(5)

	return &NestsDBStore{
		logger: logger,
		db:     db,
		dbName: config.Db,
		dsn:    dsn,
	}, nil
}
