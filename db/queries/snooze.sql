-- name: UpsertSnooze :exec
INSERT INTO snoozes (account, message, until, created)
VALUES (?, ?, ?, ?)
ON CONFLICT(account, message) DO UPDATE SET until = excluded.until;

-- name: DeleteSnooze :exec
DELETE FROM snoozes WHERE account = ? AND message = ?;

-- name: DueSnoozes :many
SELECT message FROM snoozes WHERE account = ? AND until <= ? ORDER BY until;

-- name: ListSnoozes :many
SELECT message, until FROM snoozes WHERE account = ? ORDER BY until;

-- name: RecordDone :exec
INSERT INTO done_log (account, message, at)
VALUES (?, ?, ?)
ON CONFLICT(account, message) DO UPDATE SET at = excluded.at;

-- name: CountDoneSince :one
SELECT COUNT(*) FROM done_log WHERE account = ? AND at >= ?;
