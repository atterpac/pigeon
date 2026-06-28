-- name: UpsertContact :exec
INSERT INTO contacts (account, addr, name, last_seen, freq)
VALUES (sqlc.arg(account), sqlc.arg(addr), sqlc.arg(name), sqlc.arg(last_seen), 1)
ON CONFLICT(account, addr) DO UPDATE SET
    last_seen = MAX(contacts.last_seen, excluded.last_seen),
    freq = contacts.freq + 1,
    -- Keep the display name from the most-recent message that carried one, so a
    -- later-but-older backfilled envelope doesn't clobber a fresher name.
    name = CASE
        WHEN excluded.name != '' AND excluded.last_seen >= contacts.last_seen THEN excluded.name
        ELSE contacts.name
    END;

-- name: SearchContacts :many
SELECT addr, name, last_seen, freq FROM contacts
WHERE account = sqlc.arg(account)
  AND (addr LIKE sqlc.arg(addr) OR name LIKE sqlc.arg(name))
ORDER BY freq DESC, last_seen DESC
LIMIT sqlc.arg(limit);
