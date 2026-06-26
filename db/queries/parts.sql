-- name: InsertPart :exec
INSERT INTO parts (
    account, message, seq, content_type, charset,
    disposition, filename, size, content, blob_ref
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(account, message, seq) DO UPDATE SET
    content_type = excluded.content_type,
    charset = excluded.charset,
    disposition = excluded.disposition,
    filename = excluded.filename,
    size = excluded.size,
    content = excluded.content,
    blob_ref = excluded.blob_ref;

-- name: ListParts :many
SELECT * FROM parts WHERE account = ? AND message = ? ORDER BY seq;

-- name: DeleteParts :exec
DELETE FROM parts WHERE account = ? AND message = ?;
