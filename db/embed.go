// Package db embeds the goose migration files so the store can run them at
// startup without depending on files on disk. sqlc-generated query code lives
// in internal/store/db.
package db

import "embed"

// Migrations holds the goose migration SQL.
//
//go:embed migrations/*.sql
var Migrations embed.FS
