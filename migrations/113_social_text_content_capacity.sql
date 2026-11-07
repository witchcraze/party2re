-- Letter and park body limits are deployment-configurable, and park bodies are stored HTML-escaped.
ALTER TABLE character_letters MODIFY content TEXT NOT NULL;
ALTER TABLE park_posts MODIFY content TEXT NOT NULL;

INSERT IGNORE INTO schema_migrations (version) VALUES ('113_social_text_content_capacity');
