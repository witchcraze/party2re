-- Preserve explicit Depot item order; existing rows retain their ID tie-breaker.
ALTER TABLE depot_items
    ADD COLUMN sort_position INT NOT NULL DEFAULT 0,
    ADD CONSTRAINT chk_depot_items_sort_position CHECK (sort_position >= 0);

INSERT IGNORE INTO schema_migrations (version) VALUES ('099_depot_item_order');
