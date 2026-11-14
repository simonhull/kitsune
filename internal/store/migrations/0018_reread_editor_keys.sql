-- A scan now reads every field through the same keys as the tag editor, so a
-- value the editor showed but a scan missed (a disc under DISC rather than
-- DISCNUMBER, artists only under ARTISTS, a year only under YEAR) reaches the
-- library. A quick scan reads a file again only when its size or mtime differ
-- from what is stored, so this sets the stored mtime of just the tracks that
-- may read differently to 0: a column the new keys fill is empty, and the
-- track holds one of those keys. The next scan reads them, and stores their
-- real mtime again. Nothing here is a time.

UPDATE tracks SET mtime = 0
WHERE (
	(track_no IS NULL AND id IN (SELECT track_id FROM track_tags WHERE key = 'TRACK'))
	OR (track_total IS NULL AND id IN (SELECT track_id FROM track_tags WHERE key IN ('TRACK', 'TRACKC')))
	OR (disc_no IS NULL AND id IN (SELECT track_id FROM track_tags WHERE key = 'DISC'))
	OR (disc_total IS NULL AND id IN (SELECT track_id FROM track_tags WHERE key IN ('DISC', 'DISCC')))
	OR (year IS NULL AND id IN (SELECT track_id FROM track_tags WHERE key IN ('DATE', 'YEAR', 'TDRC', 'TYER', '©day', 'ICRD')))
	OR (
		NOT EXISTS (SELECT 1 FROM track_artists WHERE track_id = tracks.id AND role = 'artist')
		AND id IN (SELECT track_id FROM track_tags WHERE key = 'ARTISTS')
	)
	OR (
		NOT EXISTS (SELECT 1 FROM track_artists WHERE track_id = tracks.id AND role = 'album_artist')
		AND id IN (SELECT track_id FROM track_tags WHERE key IN ('ALBUM_ARTIST', 'ALBUMARTISTS', 'ALBUM_ARTISTS'))
	)
);
