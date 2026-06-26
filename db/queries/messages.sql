-- name: UpsertMessage :exec
INSERT INTO messages (
    id, account, thread, subject,
    from_json, to_json, cc_json, bcc_json,
    date, snippet, category, flags, has_attachments, body_loaded,
    rfc_message_id, refs
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(account, id) DO UPDATE SET
    thread = excluded.thread,
    subject = excluded.subject,
    from_json = excluded.from_json,
    to_json = excluded.to_json,
    cc_json = excluded.cc_json,
    bcc_json = excluded.bcc_json,
    date = excluded.date,
    snippet = excluded.snippet,
    category = excluded.category,
    flags = excluded.flags,
    has_attachments = excluded.has_attachments,
    body_loaded = excluded.body_loaded,
    rfc_message_id = excluded.rfc_message_id,
    refs = excluded.refs;

-- name: GetMessage :one
SELECT * FROM messages WHERE account = ? AND id = ?;

-- name: ListThreadMessages :many
SELECT * FROM messages
WHERE account = ? AND thread = ?
ORDER BY date ASC;

-- name: ListMailboxMessages :many
SELECT m.* FROM messages m
JOIN message_labels l ON l.account = m.account AND l.message = m.id
WHERE m.account = ? AND l.label = ?
ORDER BY m.date DESC
LIMIT ?;

-- name: GetFlags :one
SELECT flags FROM messages WHERE account = ? AND id = ?;

-- name: SetFlags :exec
UPDATE messages SET flags = ? WHERE account = ? AND id = ?;

-- name: SetBodyLoaded :exec
UPDATE messages SET body_loaded = ? WHERE account = ? AND id = ?;

-- name: DeleteMessage :exec
DELETE FROM messages WHERE account = ? AND id = ?;
