bin := "email"
out := "bin/" + bin
pkg := "./cmd/email"

# goose CLI config (migrations run embedded at runtime; these are for dev)
export GOOSE_DRIVER := "sqlite3"
export GOOSE_MIGRATION_DIR := "db/migrations"
dev_db := env_var_or_default("EMAIL_DB", "dev.db")

# list recipes
default:
    @just --list

# generate sqlc query code from db/queries + db/migrations
generate:
    sqlc generate

# create a new timestamped goose migration: just new-migration add_foo
new-migration name:
    goose -dir {{GOOSE_MIGRATION_DIR}} create {{name}} sql

# apply migrations to the dev db
migrate-up:
    GOOSE_DBSTRING={{dev_db}} goose up

# roll back the last migration on the dev db
migrate-down:
    GOOSE_DBSTRING={{dev_db}} goose down

# migration status of the dev db
migrate-status:
    GOOSE_DBSTRING={{dev_db}} goose status

# build the Vue frontend into frontend/dist and copy it to cmd/email/dist
build-frontend:
    cd frontend && pnpm install && pnpm build
    rm -rf cmd/email/dist
    cp -r frontend/dist cmd/email/dist

# generate Wails frontend bindings from the Go services
bindings:
    cd cmd/email && wails3 generate bindings -ts -d ../../frontend/src/bindings

# build the CLI binary into bin/
build:
    go build -o {{out}} {{pkg}}

# run the CLI harness
run:
    just build-frontend
    go run ./cmd/email

# compile everything (no binary)
check:
    go build ./...

# run tests
test *args:
    go test ./... {{args}}

# tests with race detector + coverage
test-race:
    go test -race -cover ./...

# go vet
vet:
    go vet ./...

# format all sources
fmt:
    gofmt -w .

# verify formatting is clean (CI)
fmt-check:
    @test -z "$(gofmt -l .)" || { echo "needs gofmt:"; gofmt -l .; exit 1; }

# staticcheck (requires honnef.co/go/tools/cmd/staticcheck)
lint:
    staticcheck ./...

# tidy + verify modules
tidy:
    go mod tidy
    go mod verify

# fmt + vet + test — pre-commit gate
ci: fmt-check vet test

# remove build artifacts
clean:
    rm -rf bin
