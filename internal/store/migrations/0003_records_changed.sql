-- Rows by when they last changed, so "what changed lately" (the widget
-- Pocket Hub shows) reads only the newest rows, however many a person keeps.
-- The table and the bin mark are in it, so the rows themselves are not read.
CREATE INDEX records_changed ON records(user_id, updated_at, sheet_id, deleted_at);
