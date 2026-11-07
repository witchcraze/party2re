-- Avatars follow the legacy imgur-only contract (custom_image.cgi:65-70): clear values
-- the contract cannot produce, including data URIs from the retired upload route.
-- utf8mb4_bin keeps the match case-sensitive like the Go validator. 'chr/' values are
-- written by the God wish (legacy sets $m{icon}, not the avatar) and stay until that
-- misplacement is fixed separately; users cannot submit them.
UPDATE character_profiles
SET avatar_url = ''
WHERE avatar_url <> ''
  AND avatar_url COLLATE utf8mb4_bin NOT LIKE 'chr/%'
  AND avatar_url COLLATE utf8mb4_bin NOT REGEXP '^https://i\\.imgur\\.com/[A-Za-z0-9]{5,10}\\.gif$';

UPDATE character_presence_snapshots
SET icon_url = ''
WHERE icon_url <> ''
  AND icon_url COLLATE utf8mb4_bin NOT REGEXP '^https://i\\.imgur\\.com/[A-Za-z0-9]{5,10}\\.gif$';

INSERT IGNORE INTO schema_migrations (version) VALUES ('117_avatar_imgur_only');
