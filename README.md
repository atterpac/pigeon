# Pigeon

A keyboard-focused email client. Go + Vue, built with [Wails 3](https://v3.wails.io).

## Features

- **Vim-style keyboard control** — j/k navigation, count prefixes (`5j`), `v` visual multi-select, `:` ex-commands, `/` search, `?` help.
- **Fast triage** — archive / snooze / delete / spam / label / move, in batch, all with undo (consecutive actions collapse into one).
- **Compose** — reply / reply-all / forward with threading, draft autosave, configurable undo-send window, attachments (≤25MB), per-account HTML signatures with inline images.
- **Search** — local + server-side, operators (`from:` `to:` `subject:` `is:` `after:` `before:`), saved searches.
- **Sync** — IMAP IDLE push for the inbox, incremental UID/CONDSTORE sync, configurable poll interval, multiple accounts.
- **Notifications** — native configurable desktop alerts with modes (all / inbox-only / off), muted senders, quiet hours.
- **Privacy** — remote images blocked by default, so tracking pixels never fire on open (load per-message when you choose).
- **Customizable UI** — themes, density, and swappable compose / sidebar / nav / settings layouts.

## Build from source

### Prerequisites

- Go 1.26+
- [Wails 3](https://v3.wails.io) CLI (`wails3`)
- [`just`](https://github.com/casey/just)
- [pnpm](https://pnpm.io)
- C toolchain (CGO required for SQLite)

### Build

```sh
just build          # host binary -> bin/Pigeon
just bundle         # distributable for the host OS (see below)
just run            # build frontend + launch for local dev
```

`just bundle` dispatches by host OS: macOS `.app`, Linux AppImage, Windows NSIS installer — all into `bin/`. Cross-OS bundling isn't supported; build each target on its own OS. `just build-frontend` regenerates Wails bindings, builds the Vue app, and copies it to `cmd/email/dist`.

Run `just` (no args) for the full recipe list.

## Accounts

Pigeon is **IMAP + SMTP only — no OAuth**. OAuth would mean running a redirect server and passing Google's app-verification review; not worth it for a personal client. You authenticate with a per-app password instead.
There is maybe a day where I implement the server so others can "roll their own" OAuth, but that day is not today.

### Gmail

1. Enable 2-Step Verification on your Google account.
2. Create an **App Password** at <https://myaccount.google.com/apppasswords>.
3. Add the account in Pigeon using that 16-character password

Defaults: IMAP `imap.gmail.com:993` (TLS), SMTP `smtp.gmail.com:587` (STARTTLS). Any other provider works too — just supply its IMAP/SMTP host, ports, and an app password.
