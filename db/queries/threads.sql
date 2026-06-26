-- name: UpsertThread :exec
INSERT INTO threads (id, account, subject, last, unread)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(account, id) DO UPDATE SET
    subject = CASE WHEN threads.subject = '' THEN excluded.subject ELSE threads.subject END,
    last    = MAX(threads.last, excluded.last),
    unread  = MAX(threads.unread, excluded.unread);

-- name: ListThreads :many
SELECT id, account, subject, last, unread
FROM threads WHERE account = ?
ORDER BY last DESC
LIMIT ?;
