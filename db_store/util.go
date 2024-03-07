package db_store

import "github.com/jmoiron/sqlx"

func closeRows(rows *sqlx.Rows, origErr error) error {
	closeErr := rows.Close()
	if origErr != nil {
		closeErr = origErr
	}
	return closeErr
}
