ALTER TABLE official_model_price_versions
    ALTER COLUMN input_price_per_unit TYPE numeric(30, 18),
    ALTER COLUMN output_price_per_unit TYPE numeric(30, 18),
    ALTER COLUMN cached_input_price_per_unit TYPE numeric(30, 18),
    ALTER COLUMN reasoning_price_per_unit TYPE numeric(30, 18);
