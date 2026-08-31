ALTER TABLE price_versions
    ALTER COLUMN input_price_per_unit TYPE numeric(40, 30),
    ALTER COLUMN output_price_per_unit TYPE numeric(40, 30),
    ALTER COLUMN cached_input_price_per_unit TYPE numeric(40, 30),
    ALTER COLUMN reasoning_price_per_unit TYPE numeric(40, 30),
    ALTER COLUMN minimum_charge TYPE numeric(40, 30);

ALTER TABLE official_model_price_versions
    ALTER COLUMN input_price_per_unit TYPE numeric(40, 30),
    ALTER COLUMN output_price_per_unit TYPE numeric(40, 30),
    ALTER COLUMN cached_input_price_per_unit TYPE numeric(40, 30),
    ALTER COLUMN reasoning_price_per_unit TYPE numeric(40, 30);

ALTER TABLE price_components
    ALTER COLUMN price_per_unit TYPE numeric(40, 30);

ALTER TABLE official_price_components
    ALTER COLUMN price_per_unit TYPE numeric(40, 30);

ALTER TABLE model_requests
    ALTER COLUMN estimated_amount TYPE numeric(40, 30),
    ALTER COLUMN settled_amount TYPE numeric(40, 30);

ALTER TABLE billing_reservations
    ALTER COLUMN reserved_amount TYPE numeric(40, 30),
    ALTER COLUMN settled_amount TYPE numeric(40, 30),
    ALTER COLUMN released_amount TYPE numeric(40, 30);

ALTER TABLE ledger_accounts
    ALTER COLUMN balance TYPE numeric(40, 30);

ALTER TABLE ledger_lines
    ALTER COLUMN amount TYPE numeric(40, 30);
