app := "Pigeon"
# host GOARCH (map just's arch() to Go naming) and host binary extension
goarch := if arch() == "x86_64" { "amd64" } else if arch() == "aarch64" { "arm64" } else { arch() }
ext := if os() == "windows" { ".exe" } else { "" }
out := "bin/" + app + ext
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

# build a universal (arm64 + amd64) macOS binary into bin/ via lipo. Apple's
# clang/SDK are cross-arch, so the x86_64 slice builds fine on Apple Silicon.
[macos]
build-universal:
    MACOSX_DEPLOYMENT_TARGET=12.0 CGO_CFLAGS=-mmacosx-version-min=12.0 CGO_LDFLAGS=-mmacosx-version-min=12.0 CGO_ENABLED=1 GOARCH=arm64 go build -buildvcs=false -gcflags=all="-l" -o bin/{{app}}-arm64 {{pkg}}
    MACOSX_DEPLOYMENT_TARGET=12.0 CGO_CFLAGS=-mmacosx-version-min=12.0 CGO_LDFLAGS=-mmacosx-version-min=12.0 CGO_ENABLED=1 GOARCH=amd64 go build -buildvcs=false -gcflags=all="-l" -o bin/{{app}}-amd64 {{pkg}}
    lipo -create -output {{out}} bin/{{app}}-arm64 bin/{{app}}-amd64
    rm bin/{{app}}-arm64 bin/{{app}}-amd64

# cross-compile a Windows .exe from any host (pure Go)
xbuild-windows arch="amd64": build-frontend
    GOOS=windows GOARCH={{arch}} CGO_ENABLED=0 go build -buildvcs=false -ldflags="-H windowsgui" -o bin/{{app}}-windows-{{arch}}.exe {{pkg}}

# generate platform icon assets from build/appicon.png; remove stale Assets.car
# so macOS loads CFBundleIconFile=icons from icons.icns.
generate-icons:
    rm -f build/darwin/Assets.car
    cd build && wails3 generate icons -input appicon.png -macfilename darwin/icons.icns -windowsfilename windows/icon.ico

# create the macOS development .app wrapper used by local runs
[macos]
bundle-dev: build-frontend build generate-icons
    rm -rf {{dev_app}}
    mkdir -p {{dev_app}}/Contents/MacOS
    mkdir -p {{dev_app}}/Contents/Resources
    cp build/darwin/icons.icns {{dev_app}}/Contents/Resources/
    if [ -f build/darwin/Assets.car ]; then cp build/darwin/Assets.car {{dev_app}}/Contents/Resources/; fi
    cp {{out}} {{dev_app}}/Contents/MacOS/
    cp build/darwin/Info.dev.plist {{dev_app}}/Contents/Info.plist
    codesign --force --deep --sign - {{dev_app}}

# package a distributable bundle for the host OS: macOS .app / Linux AppImage / Windows NSIS
[macos]
bundle: build-frontend build-universal generate-icons
    rm -rf {{release_app}}
    mkdir -p {{release_app}}/Contents/MacOS
    mkdir -p {{release_app}}/Contents/Resources
    cp build/darwin/icons.icns {{release_app}}/Contents/Resources/
    if [ -f build/darwin/Assets.car ]; then cp build/darwin/Assets.car {{release_app}}/Contents/Resources/; fi
    cp {{out}} {{release_app}}/Contents/MacOS/
    cp build/darwin/Info.plist {{release_app}}/Contents/Info.plist
    codesign --force --deep --sign - {{release_app}}

# Linux: AppImage -> bin/ (binary is self-contained; AppImage just wraps it)
[linux]
bundle: build-frontend build
    wails3 generate .desktop -name "{{app}}" -exec "{{app}}" -icon "{{app}}" -outputfile build/linux/{{app}}.desktop -categories "Network;Email;"
    cp {{out}} build/linux/appimage/{{app}}
    cp build/appicon.png build/linux/appimage/{{app}}.png
    wails3 generate appimage -binary build/linux/appimage/{{app}} -icon build/linux/appimage/{{app}}.png -desktopfile build/linux/{{app}}.desktop -outputdir bin -builddir build/linux/appimage/build

# Windows: embed icon/manifest via .syso, build, then NSIS installer (needs makensis)
[windows]
bundle: build-frontend generate-icons
    wails3 generate syso -arch {{goarch}} -icon build/windows/icon.ico -manifest build/windows/wails.exe.manifest -info build/windows/info.json -out cmd/email/wails_windows_{{goarch}}.syso
    just build
    rm -f cmd/email/wails_windows_{{goarch}}.syso
    wails3 generate webview2bootstrapper -dir build/windows/nsis
    cd build/windows/nsis && makensis -DINFO_PROJECTNAME={{app}} -DINFO_PRODUCTNAME={{app}} -DINFO_COMPANYNAME=Atterpac -DARG_WAILS_{{uppercase(goarch)}}_BINARY="{{justfile_directory()}}\{{replace(out, '/', '\')}}" project.nsi

# build every distributable format available for the host OS (mac: .app + .dmg / linux: AppImage + deb + rpm + arch / win: NSIS)
[macos]
bundle-full: bundle
    # .dmg built with hdiutil; Wails' own dmg packager is disabled in this version
    rm -rf bin/dmg bin/{{app}}.dmg
    mkdir -p bin/dmg
    cp -R {{release_app}} bin/dmg/
    ln -s /Applications bin/dmg/Applications
    hdiutil create -volname {{app}} -srcfolder bin/dmg -ov -format UDZO bin/{{app}}.dmg
    rm -rf bin/dmg

# Linux: AppImage (from bundle) + deb + rpm + archlinux (nfpm, no extra tooling).
[linux]
bundle-full: bundle
    GOARCH={{goarch}} wails3 tool package -name {{app}} -format deb -config build/linux/nfpm/nfpm.yaml -out bin
    GOARCH={{goarch}} wails3 tool package -name {{app}} -format rpm -config build/linux/nfpm/nfpm.yaml -out bin
    GOARCH={{goarch}} wails3 tool package -name {{app}} -format archlinux -config build/linux/nfpm/nfpm.yaml -out bin

# Windows: NSIS installer only — this Wails version ships no msix tooling.
[windows]
bundle-full: bundle
    @echo "Windows: NSIS installer only (no msix tool in this Wails version)"

# run the app for local dev (macOS uses the .app so notifications get a bundle id; Linux/Windows exec the binary)
[macos]
run: bundle-dev
    {{dev_app}}/Contents/MacOS/{{app}}

[linux]
run: build-frontend build
    {{out}}

[windows]
run: build-frontend build
    {{out}}

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
