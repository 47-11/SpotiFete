BEGIN;
ALTER TABLE listening_sessions
    DROP COLUMN fallback_playlist;
COMMIT;
