-- Where an imported base came from (a Pocket Tables export's base, or a
-- spreadsheet's content hash), so bringing the same file in again adds
-- nothing.
ALTER TABLE bases ADD COLUMN import_ref TEXT NOT NULL DEFAULT '';
CREATE INDEX bases_import_ref ON bases(user_id, import_ref) WHERE import_ref <> '';
