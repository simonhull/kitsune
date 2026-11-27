import { describe, expect, it } from 'vitest';
import type { MetadataSource, MetadataStatus } from './types.ts';
import {
	albumsWords,
	chipLeadsToKey,
	chipWords,
	eyebrowWords,
	failureWords,
	figureWords,
	keyPlace,
	keysWords,
	noMbidWords,
	nowWords,
	offerTitle,
	passShare,
	restWords,
	sourceWhat
} from './words.ts';

const source = (over: Partial<MetadataSource> = {}): MetadataSource => ({
	name: 'fanarttv',
	enabled: true,
	state: 'on',
	key: 'set',
	clientKey: 'missing',
	...over
});

const artists = {
	albumArtists: 418,
	withMbid: 351,
	lookedUp: 212,
	withArt: 371,
	foundNothing: 47,
	withoutMbid: 67
};

const status = (over: Partial<MetadataStatus> = {}): MetadataStatus => ({
	remote: true,
	language: 'en',
	pass: { running: false, phase: 'artists', done: 0, total: 0 },
	sources: [source()],
	artists,
	albums: { albums: 931, withMbid: 805, fromArchive: 0, withoutCover: 0 },
	...over
});

describe('the Metadata card in words', () => {
	it('says off, fetching, up to date, or that no source is on', () => {
		expect(eyebrowWords(status({ remote: false }))).toBe('ARTIST ART AND BIOS · OFF');
		expect(
			eyebrowWords(status({ pass: { running: true, phase: 'artists', done: 1, total: 4 } }))
		).toBe('ARTIST ART AND BIOS · FETCHING');
		expect(eyebrowWords(status())).toBe('ARTIST ART AND BIOS · UP TO DATE');
		expect(eyebrowWords(status({ sources: [source({ state: 'resting' })] }))).toBe(
			'ARTIST ART AND BIOS · UP TO DATE'
		);
		expect(eyebrowWords(status({ sources: [source({ state: 'needsKey' })] }))).toBe(
			'ARTIST ART AND BIOS · NO SOURCE ON'
		);
	});

	it('offers lookups for the artists with an MBID', () => {
		expect(offerTitle(1351)).toBe('Fetch art and bios for 1,351 artists?');
		expect(offerTitle(1)).toBe('Fetch art and bios for 1 artist?');
		expect(offerTitle(0)).toBe('Fetch art and bios for your artists?');
	});

	it('says whether fanart.tv has its key, never what it is', () => {
		expect(keysWords(source({ key: 'missing' }), false)).toBe(
			'FANART.TV NEEDS YOURS · SERVER › SETTINGS'
		);
		expect(keysWords(source(), true)).toBe('FANART.TV’S KEY IS SET');
		expect(keysWords(undefined, true)).toBe('FANART.TV NEEDS YOURS');
	});

	it('counts what is looked up while a pass goes, and what has art after', () => {
		expect(
			figureWords(status({ pass: { running: true, phase: 'artists', done: 3, total: 9 } }))
		).toEqual({
			figure: '212',
			of: 'of 351 artists looked up'
		});
		expect(figureWords(status())).toEqual({ figure: '371', of: 'of 418 artists have art' });
		expect(figureWords(status({ artists: { ...artists, albumArtists: 1, withArt: 1 } })).of).toBe(
			'of 1 artist has art'
		);
	});

	it('says how far the pass is and who is in hand', () => {
		expect(passShare({ running: true, phase: 'artists', done: 3, total: 12 })).toBe(0.25);
		expect(passShare({ running: false, phase: 'artists', done: 0, total: 0 })).toBe(0);
		const pass = {
			running: true,
			phase: 'artists' as const,
			done: 3,
			total: 1200,
			artist: { id: 'a', name: 'Mere Coast' }
		};
		expect(nowWords(pass, false)).toBe('NOW · MERE COAST · 3 OF 1,200 · MUSIC KEEPS PLAYING');
		expect(nowWords({ running: true, phase: 'artists', done: 0, total: 4 }, true)).toBe(
			'NOW · 0 OF 4'
		);
		const albums = {
			running: true,
			phase: 'albums' as const,
			done: 12,
			total: 96,
			album: { id: 'b', name: 'Tidelines' }
		};
		expect(nowWords(albums, false)).toBe('NOW · TIDELINES · 12 OF 96 ALBUMS · MUSIC KEEPS PLAYING');
	});

	it('says how many covers came from the Cover Art Archive, and how many albums have none', () => {
		const albums = { albums: 931, withMbid: 805, fromArchive: 23, withoutCover: 12 };
		expect(albumsWords(albums, false)).toBe(
			'23 ALBUM COVERS FROM THE COVER ART ARCHIVE · 12 ALBUMS HAVE NO COVER'
		);
		expect(albumsWords(albums, true)).toBe('23 COVERS FROM THE ARCHIVE');
		expect(albumsWords({ ...albums, fromArchive: 1, withoutCover: 1 }, false)).toBe(
			'1 ALBUM COVER FROM THE COVER ART ARCHIVE · 1 ALBUM HAS NO COVER'
		);
		expect(albumsWords({ ...albums, fromArchive: 0, withoutCover: 0 }, false)).toBeNull();
	});

	it('says what found nothing and when the next refresh is', () => {
		expect(restWords(artists, '2026-12-18T12:00:00Z', false)).toBe(
			'47 FOUND NOTHING YET · ASKED AGAIN LATER · NEXT REFRESH DEC 18'
		);
		expect(restWords(artists, '2026-12-18T12:00:00Z', true)).toBe('NEXT REFRESH DEC 18');
		expect(restWords({ ...artists, foundNothing: 0 }, undefined, false)).toBe(
			'EVERY ARTIST WITH AN ID ASKED ABOUT'
		);
	});

	it('words each state, and leads a key’s trouble to Settings', () => {
		expect(chipWords(source())).toBe('ON');
		expect(chipWords(source({ state: 'off' }))).toBe('OFF');
		expect(chipWords(source({ state: 'needsKey' }))).toBe('NEEDS A KEY');
		expect(chipWords(source({ state: 'refused' }))).toBe('REFUSED THE KEY');
		expect(chipWords(source({ state: 'resting' }))).toBe('RESTING');
		const until = new Date(2026, 10, 23, 22, 40).toISOString();
		expect(chipWords(source({ state: 'resting', until }))).toBe('RESTING · AGAIN 22:40');
		expect(chipLeadsToKey(source({ state: 'needsKey' }))).toBe(true);
		expect(chipLeadsToKey(source({ state: 'refused' }))).toBe(true);
		expect(chipLeadsToKey(source({ state: 'resting' }))).toBe(false);
		expect(keyPlace(source())).toBe('/server#art');
		expect(keyPlace(source({ name: 'lastfm', state: 'needsKey' }))).toBe('/server#lastfm');
	});

	it('gives a failure only in the server’s words, and only while it matters', () => {
		const now = new Date(2026, 10, 23, 15, 0).getTime();
		const failure = {
			words: 'fanart.tv refused the key',
			at: new Date(2026, 10, 23, 14, 2).toISOString()
		};
		expect(failureWords(source({ state: 'refused', failure }), now)).toBe(
			'fanart.tv refused the key · TODAY · 14:02'
		);
		expect(failureWords(source({ failure }), now)).toBe(
			'Last trouble: fanart.tv refused the key · TODAY · 14:02'
		);
		expect(failureWords(source({ state: 'off', failure }), now)).toBeNull();
		expect(failureWords(source({ state: 'needsKey', failure }), now)).toBeNull();
		expect(failureWords(source({ state: 'refused' }), now)).toBeNull();
	});

	it('says what a personal key adds, and leads artists without an MBID to Matching', () => {
		expect(sourceWhat(source({ clientKey: 'set' }))).toBe(
			'thumbs, backdrops, logos, banners and disc art · your own key too'
		);
		expect(sourceWhat(source({ name: 'coverartarchive', key: 'none', clientKey: 'none' }))).toBe(
			'covers your files lack, back covers · no key'
		);
		expect(sourceWhat(source({ name: 'wikipedia', key: 'none', clientKey: 'none' }))).toBe(
			'bios and portraits · no key'
		);
		expect(noMbidWords(67, false)).toBe(
			'67 ALBUM ARTISTS HAVE NO MUSICBRAINZ ID · MATCH THEIR ALBUMS TO FETCH THEIR ART'
		);
		expect(noMbidWords(1, true)).toBe('1 ALBUM ARTIST HAS NO MUSICBRAINZ ID · MATCH THEIR ALBUMS');
	});
});
