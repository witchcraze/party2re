-- Roster order is joined_at, then character ID; second precision let rows
-- written in the same second fall back to ID order instead of join order.
ALTER TABLE guild_members
    MODIFY joined_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6);

INSERT IGNORE INTO schema_migrations (version) VALUES ('150_guild_member_joined_at_precision');
