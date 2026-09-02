-- Cost factor used to estimate the upstream provider charge for a channel.
-- 1.000000 keeps the official reference price unchanged; 0.500000 means
-- the upstream contract cost is estimated at 50% of the reference price.

ALTER TABLE channels
    ADD COLUMN IF NOT EXISTS upstream_cost_discount numeric(30, 18) NOT NULL DEFAULT 1
        CHECK (upstream_cost_discount >= 0 AND upstream_cost_discount <= 1000);
