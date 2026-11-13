-- Stars are the only like (plan 061). What a heart counted as is gone with it,
-- so the pref that said so goes. Last.fm loves follow stars from here: a song
-- is loved once its owner rates it love_from halves or more, 8 (4★) unless
-- they choose. Hearts stay where they are until each person turns them into
-- stars, or not, through POST /me/hearts/convert.

DELETE FROM user_prefs WHERE key = 'ratings.heart_stars';

ALTER TABLE lastfm_accounts ADD COLUMN love_from INTEGER NOT NULL DEFAULT 8 CHECK (love_from BETWEEN 1 AND 10);
