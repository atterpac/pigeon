app := "Pigeon"
out := "bin/" + app
pkg := "./cmd/email"
dev_app := "bin/" + app + ".dev.app"
release_app := "bin/" + app + ".app"

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
    just bindings
    cd frontend && if [ ! -d node_modules ]; then pnpm install --frozen-lockfile; fi && pnpm build-only
    rm -rf cmd/email/dist
    cp -r frontend/dist cmd/email/dist

# generate Wails frontend bindings from the Go services
bindings:
    cd cmd/email && wails3 generate bindings -ts -d ../../frontend/src/bindings

# build the desktop binary into bin/
build:
    CGO_ENABLED=1 go build -buildvcs=false -gcflags=all="-l" -o {{out}} {{pkg}}

# generate platform icon assets from build/appicon.png; remove stale Assets.car
# so macOS loads CFBundleIconFile=icons from icons.icns.
generate-icons:
    rm -f build/darwin/Assets.car
    cd build && wails3 generate icons -input appicon.png -macfilename darwin/icons.icns -windowsfilename windows/icon.ico

# create the macOS development .app wrapper used by local runs
bundle-dev: build generate-icons
    rm -rf {{dev_app}}
    mkdir -p {{dev_app}}/Contents/MacOS
    mkdir -p {{dev_app}}/Contents/Resources
    cp build/darwin/icons.icns {{dev_app}}/Contents/Resources/
    if [ -f build/darwin/Assets.car ]; then cp build/darwin/Assets.car {{dev_app}}/Contents/Resources/; fi
    cp {{out}} {{dev_app}}/Contents/MacOS/
    cp build/darwin/Info.dev.plist {{dev_app}}/Contents/Info.plist
    codesign --force --deep --sign - {{dev_app}}

# create the macOS release .app wrapper
bundle: build generate-icons
    rm -rf {{release_app}}
    mkdir -p {{release_app}}/Contents/MacOS
    mkdir -p {{release_app}}/Contents/Resources
    cp build/darwin/icons.icns {{release_app}}/Contents/Resources/
    if [ -f build/darwin/Assets.car ]; then cp build/darwin/Assets.car {{release_app}}/Contents/Resources/; fi
    cp {{out}} {{release_app}}/Contents/MacOS/
    cp build/darwin/Info.plist {{release_app}}/Contents/Info.plist
    codesign --force --deep --sign - {{release_app}}

# run the macOS app bundle; required for notifications because macOS reads
# CFBundleIdentifier from the app bundle, not from a bare `go run` process.
run: build-frontend bundle-dev
    {{dev_app}}/Contents/MacOS/{{app}}

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
