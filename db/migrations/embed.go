// Package migrations embeds the goose migration SQL so the store can run them
// at startup without depending on files on disk. sqlc-generated query code lives
// in internal/store/db.
package migrations

import "embed"

// FS holds the goose migration SQL, rooted at the package directory.
//
//go:embed *.sql
var FS embed.FS
