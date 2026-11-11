-- Ratings in half stars: 1 (½★) to 10 (5★). Every rating doubles, exactly:
-- 1★ becomes 2 and 5★ becomes 10. rated_at is kept as it was. The column is
-- renamed so nothing can go on reading halves as whole stars.

CREATE TABLE track_ratings_new (
	user_id  INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	track_id INTEGER NOT NULL REFERENCES tracks (id) ON DELETE CASCADE,
	halves   INTEGER NOT NULL CHECK (halves BETWEEN 1 AND 10),
	rated_at INTEGER NOT NULL, -- the latest rating
	PRIMARY KEY (user_id, track_id)
) STRICT, WITHOUT ROWID;

INSERT INTO track_ratings_new (user_id, track_id, halves, rated_at)
SELECT user_id, track_id, stars * 2, rated_at FROM track_ratings;

DROP TABLE track_ratings;
ALTER TABLE track_ratings_new RENAME TO track_ratings;

CREATE INDEX track_ratings_track ON track_ratings (track_id);
