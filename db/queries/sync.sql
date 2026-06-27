-- name: SetCursor :exec
INSERT INTO sync_state (account, mailbox, cursor)
VALUES (?, ?, ?)
ON CONFLICT(account, mailbox) DO UPDATE SET cursor = excluded.cursor;

-- name: GetCursor :one
SELECT cursor FROM sync_state WHERE account = ? AND mailbox = ?;

-- name: SetBackfill :exec
INSERT INTO sync_state (account, mailbox, backfill)
VALUES (?, ?, ?)
ON CONFLICT(account, mailbox) DO UPDATE SET backfill = excluded.backfill;

-- name: GetBackfill :one
SELECT backfill FROM sync_state WHERE account = ? AND mailbox = ?;

-- name: GetBackfillState :one
SELECT backfill, backfill_done FROM sync_state WHERE account = ? AND mailbox = ?;

-- name: MarkBackfillDone :exec
INSERT INTO sync_state (account, mailbox, backfill, backfill_done)
VALUES (?, ?, NULL, 1)
ON CONFLICT(account, mailbox) DO UPDATE SET backfill = NULL, backfill_done = 1;

-- name: ClearCursor :exec
DELETE FROM sync_state WHERE account = ? AND mailbox = ?;

-- name: EnqueueOp :exec
INSERT INTO op_log (account, op, payload, next_at, created)
VALUES (?, ?, ?, ?, ?);

-- name: ReadyOps :many
SELECT id, account, op, payload, attempts, next_at, created
FROM op_log
WHERE account = ? AND next_at <= ?
ORDER BY id
LIMIT ?;

-- name: BumpOp :exec
UPDATE op_log SET attempts = attempts + 1, next_at = ? WHERE id = ?;

-- name: DeleteOp :exec
DELETE FROM op_log WHERE id = ?;
