-- A Navidrome database for tests: schema.sql's tables, plus the columns and
-- tables that hold secrets, so a test can put sentinels in them and check
-- none ever comes out.

CREATE TABLE library (
    id   INTEGER PRIMARY KEY,
    name TEXT NOT NULL DEFAULT '',
    path TEXT NOT NULL
);

CREATE TABLE media_file (
    id                   varchar(255) NOT NULL PRIMARY KEY,
    path                 varchar(255) NOT NULL,
    title                varchar(255) NOT NULL DEFAULT '',
    album_id             varchar(255) NOT NULL DEFAULT '',
    mbz_recording_id     varchar      NOT NULL DEFAULT '',
    mbz_release_track_id varchar      NOT NULL DEFAULT '',
    library_id           INTEGER      NOT NULL DEFAULT 1,
    missing              boolean      NOT NULL DEFAULT false
);

CREATE TABLE "user" (
    id        varchar(255) NOT NULL PRIMARY KEY,
    user_name varchar(255) NOT NULL,
    name      varchar(255) NOT NULL DEFAULT '',
    email     varchar(255) NOT NULL DEFAULT '',
    password  varchar(255) NOT NULL DEFAULT ''
);

CREATE TABLE user_props (
    user_id varchar(255) NOT NULL,
    key     varchar(255) NOT NULL,
    value   varchar(255),
    PRIMARY KEY (user_id, key)
);

CREATE TABLE annotation (
    user_id    varchar(255) NOT NULL,
    item_id    varchar(255) NOT NULL,
    item_type  varchar(255) NOT NULL,
    play_count INTEGER DEFAULT 0,
    play_date  datetime,
    rating     INTEGER DEFAULT 0,
    starred    bool    NOT NULL DEFAULT false,
    starred_at datetime,
    rated_at   datetime,
    UNIQUE (user_id, item_id, item_type)
);

CREATE TABLE scrobbles (
    id              INTEGER PRIMARY KEY,
    media_file_id   VARCHAR(255) NOT NULL,
    user_id         VARCHAR(255) NOT NULL,
    submission_time INTEGER      NOT NULL
);

CREATE TABLE artist (
    id            varchar(255) NOT NULL PRIMARY KEY,
    name          varchar(255) NOT NULL,
    mbz_artist_id varchar      NOT NULL DEFAULT ''
);
