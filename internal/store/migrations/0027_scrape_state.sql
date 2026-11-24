-- What the artist pass (plan 072, Phase 2) has asked each source, and when to
-- ask again. Nothing here is a value derived in Go from a row: the pass fills
-- it as it goes, so a new database and an old one both start empty.
--
-- scrape_state is keyed by the MusicBrainz ID asked, not by a row, so a renamed
-- or re-created artist isn't fetched again. kind is 'artist' now and 'album'
-- for a later phase. status is what the last answer was: 'found' (something to
-- show), 'none' (a 404, or nothing usable) or 'failed' (a server error, a
-- timeout, a body that doesn't parse); failures counts the failures in a row,
-- for the backoff, and error says the last one in words, never with a URL or a
-- key. next_at is when the pass asks again; fetched_at when it last asked.
--
-- scrape_feeds remembers, per source, when a feed of changes was last read
-- (fanart.tv's latest), so a restart doesn't read it again within the day.

CREATE TABLE scrape_state (
	source     TEXT    NOT NULL CHECK (source IN ('fanarttv', 'wikimedia')),
	kind       TEXT    NOT NULL CHECK (kind IN ('artist', 'album')),
	mbid       TEXT    NOT NULL CHECK (length(mbid) = 36),
	status     TEXT    NOT NULL CHECK (status IN ('found', 'none', 'failed')),
	fetched_at INTEGER NOT NULL,
	next_at    INTEGER NOT NULL,
	failures   INTEGER NOT NULL CHECK (failures >= 0),
	error      TEXT,
	PRIMARY KEY (source, kind, mbid),
	CHECK ((status = 'failed') = (failures > 0))
) STRICT, WITHOUT ROWID;

CREATE INDEX scrape_state_mbid ON scrape_state (kind, mbid);

CREATE TABLE scrape_feeds (
	source  TEXT    PRIMARY KEY CHECK (source IN ('fanarttv')),
	read_at INTEGER NOT NULL
) STRICT, WITHOUT ROWID;
