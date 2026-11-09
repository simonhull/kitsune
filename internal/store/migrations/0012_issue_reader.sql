-- The tag reader that judged a file corrupted or unsupported, as its version.
-- A scan reads such a file again once the reader differs, though the file
-- hasn't changed: a newer reader may take it. Issues from before this file have
-- none, so the next scan reads them all once.

ALTER TABLE scan_issues ADD COLUMN reader TEXT NOT NULL DEFAULT '';
