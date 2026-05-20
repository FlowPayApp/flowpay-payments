package repository

import "database/sql"

type DB struct {
	sql *sql.DB
}

func New(db *sql.DB) *DB {
	return &DB{sql: db}
}
