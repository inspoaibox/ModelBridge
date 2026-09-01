-- Separate subject namespace for sensitive-operation MFA failures. The
-- application stores only a deterministic hash of the user ID and reuses the
-- existing throttle table so no TOTP code is ever persisted.
CREATE INDEX IF NOT EXISTS login_throttles_locked_until_idx
    ON login_throttles (locked_until)
    WHERE locked_until IS NOT NULL;
