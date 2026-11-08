-- A disc's name, as its tracks are tagged: DISCSUBTITLE, or TSST on ID3v2.4.
-- NULL is untagged. It is the trimmed first value of the scan's candidates, TSST
-- then DISCSUBTITLE, so tracks scanned before this file get theirs from the raw
-- tags already stored. SQL trims only spaces, tabs and line breaks, where a scan
-- trims all white space; the next scan of a changed file settles the difference.

ALTER TABLE tracks ADD COLUMN disc_subtitle TEXT;

UPDATE tracks SET disc_subtitle = (
	SELECT nullif(trim(tt.value, ' ' || char(9, 10, 13)), '')
	FROM track_tags tt
	WHERE tt.track_id = tracks.id AND tt.key IN ('TSST', 'DISCSUBTITLE') AND tt.ord = 0 AND tt.value <> ''
	ORDER BY tt.key = 'DISCSUBTITLE'
	LIMIT 1
);
