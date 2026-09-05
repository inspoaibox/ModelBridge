-- Non-token groups use an explicit customer price for their selected meter.
-- Token groups retain the existing model price matrix and use zero here.
ALTER TABLE routing_groups
    ADD COLUMN IF NOT EXISTS metering_price numeric(30, 12) NOT NULL DEFAULT 0;

UPDATE routing_groups
SET metering_price = 0
WHERE metering_mode = 'token' AND (metering_price IS NULL OR metering_price < 0);

ALTER TABLE routing_groups
    DROP CONSTRAINT IF EXISTS routing_groups_metering_price_check;

ALTER TABLE routing_groups
    ADD CONSTRAINT routing_groups_metering_price_check
    CHECK (metering_price >= 0);
