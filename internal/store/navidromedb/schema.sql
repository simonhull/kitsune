-- The part of Navidrome's schema the importer reads, as Navidrome declares it
-- (goose version 20260922230428). sqlc generates the queries against it, and
-- Open checks that a real file has every column named here. Nothing else in a
-- Navidrome database is ever selected: user.password, user.email and
-- user_props hold secrets.

CREATE TABLE library (
    id   INTEGER PRIMARY KEY,
    path TEXT NOT NULL
);

CREATE TABLE media_file (
    id                   varchar(255) NOT NULL PRIMARY KEY,
    path                 varchar(255) NOT NULL, -- relative to its library's path
    album_id             varchar(255) NOT NULL,
    mbz_recording_id     varchar      NOT NULL,
    mbz_release_track_id varchar      NOT NULL,
    library_id           INTEGER      NOT NULL,
    missing              boolean      NOT NULL
);

CREATE TABLE "user" (
    id        varchar(255) NOT NULL PRIMARY KEY,
    user_name varchar(255) NOT NULL
);

CREATE TABLE annotation (
    user_id    varchar(255) NOT NULL,
    item_id    varchar(255) NOT NULL,
    item_type  varchar(255) NOT NULL, -- media_file, album or artist
    play_count INTEGER,
    play_date  datetime,
    rating     INTEGER,               -- 0 to 5; 0 is none
    starred    bool         NOT NULL,
    starred_at datetime,
    rated_at   datetime
);

-- Optional: an older Navidrome has no per-play history.
CREATE TABLE scrobbles (
    id              INTEGER PRIMARY KEY,
    media_file_id   VARCHAR(255) NOT NULL,
    user_id         VARCHAR(255) NOT NULL,
    submission_time INTEGER      NOT NULL -- Unix seconds
);

CREATE TABLE artist (
    id            varchar(255) NOT NULL PRIMARY KEY,
    name          varchar(255) NOT NULL,
    mbz_artist_id varchar      NOT NULL
);

-- Optional, and only counted: Kitsune has no playlists or resume points yet,
-- so the report says what stays behind.
CREATE TABLE playlist (
    id       varchar(255) NOT NULL PRIMARY KEY,
    owner_id varchar(255) NOT NULL
);

CREATE TABLE bookmark (
    user_id varchar(255) NOT NULL,
    item_id varchar(255) NOT NULL
);
