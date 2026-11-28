-- Legacy free.cgi number_select: each active listing shows the lowest free number.
ALTER TABLE fleamarket_listings ADD COLUMN listing_no INT NOT NULL DEFAULT 0 AFTER id;

SET @listing_no := 0;
UPDATE fleamarket_listings SET listing_no = (@listing_no := @listing_no + 1)
WHERE status = 'active' ORDER BY created_at, id;

INSERT IGNORE INTO schema_migrations (version) VALUES ('151_fleamarket_listing_number');
