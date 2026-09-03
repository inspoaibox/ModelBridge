ALTER TABLE api_tokens
    ADD COLUMN IF NOT EXISTS spend_limit numeric(40, 30) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS spent_amount numeric(40, 30) NOT NULL DEFAULT 0;

ALTER TABLE api_tokens
    ADD CONSTRAINT api_tokens_spend_limit_check
        CHECK (spend_limit >= 0),
    ADD CONSTRAINT api_tokens_spent_amount_check
        CHECK (spent_amount >= 0);

-- Preserve historical customer charges when the quota columns are introduced.
UPDATE api_tokens tok
SET spent_amount = COALESCE((
    SELECT SUM(mr.settled_amount)
    FROM model_requests mr
    WHERE mr.token_id = tok.id
      AND mr.status = 'settled'
), 0);

CREATE INDEX IF NOT EXISTS api_tokens_spend_limit_idx
    ON api_tokens (id, spend_limit, spent_amount)
    WHERE status = 'active' AND deleted_at IS NULL;
