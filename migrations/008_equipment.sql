CREATE TABLE IF NOT EXISTS equipment_slots (
    character_id CHAR(32) NOT NULL,
    slot VARCHAR(32) NOT NULL,
    instance_id CHAR(32) NOT NULL,
    PRIMARY KEY (character_id, slot),
    CONSTRAINT fk_equipment_slots_character
        FOREIGN KEY (character_id) REFERENCES characters (id),
    CONSTRAINT fk_equipment_slots_item
        FOREIGN KEY (instance_id) REFERENCES inventory_items (id)
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('008_equipment');
