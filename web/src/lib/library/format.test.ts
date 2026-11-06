import { describe, expect, it } from 'vitest';
import {
	albumHoldings,
	albumsLine,
	albumFacts,
	artistCount,
	artistFacts,
	artistHoldings,
	balancedLines,
	clock,
	credit,
	formatLine,
	formatOf,
	gain,
	isMultiDisc,
	libraryLine,
	trackGain,
	trackLabel
} from './format';
import type { Album, Track } from './source';

function track(fields: Partial<Track> = {}): Track {
	return {
		id: 't',
		title: 'T',
		artists: ['A'],
		durationMs: 1000,
		format: 'FLAC',
		genres: [],
		etag: '',
		...fields
	};
}

const album: Album = {
	id: 'a',
	title: 'Album',
	artist: 'A',
	artists: [],
	trackCount: 2,
	compilation: false,
	tracks: [track(), track()],
	durationMs: 1_500_000,
	formats: ['FLAC'],
	addedAt: '2026-10-12T18:00:00Z',
	genres: [],
	etag: ''
};

describe('clock', () => {
	it('pads seconds', () => {
		expect(clock(102)).toBe('1:42');
		expect(clock(61)).toBe('1:01');
	});
});

describe('gain', () => {
	it('uses a true minus sign', () => {
		expect(gain(-6.2)).toBe('−6.2');
		expect(gain(1)).toBe('+1.0');
	});
});

describe('balancedLines', () => {
	it('breaks where the board does', () => {
		expect(balancedLines('Salt in the Lungs')).toEqual(['Salt in', 'the Lungs']);
	});

	it('leaves a single word alone', () => {
		expect(balancedLines('Panes')).toEqual(['Panes', '']);
	});
});

describe('trackLabel', () => {
	it('pads a plain number and marks the disc only on a multi-disc album', () => {
		expect(trackLabel(track({ trackNo: 3 }), false)).toBe('03');
		expect(trackLabel(track({ discNo: 2, trackNo: 3 }), true)).toBe('2-03');
		expect(trackLabel(track({ trackNo: 3 }), true)).toBe('1-03');
	});

	it('invents nothing for a track with no number', () => {
		expect(trackLabel(track(), false)).toBe('–');
	});
});

describe('isMultiDisc', () => {
	it('counts an unset disc as disc 1', () => {
		expect(isMultiDisc([track({ discNo: 1 }), track()])).toBe(false);
		expect(isMultiDisc([track(), track({ discNo: 2 })])).toBe(true);
	});
});

describe('credit', () => {
	it('says nothing when the track is just the album artist', () => {
		expect(credit(track({ artists: ['A'] }), 'A')).toBeUndefined();
		expect(credit(track({ artists: [] }), 'A')).toBeUndefined();
	});

	it('names guests beside the album artist', () => {
		expect(credit(track({ artists: ['A', 'B', 'C'] }), 'A')).toBe('feat. B, C');
	});

	it('shows a track that does not name the album artist plainly', () => {
		expect(credit(track({ artists: ['B', 'C'] }), 'Various Artists')).toBe('B, C');
	});
});

describe('trackGain', () => {
	it('means the track gain, and nothing else', () => {
		expect(trackGain(track({ replayGain: { trackGain: -6.2 } }))).toBe(-6.2);
		expect(trackGain(track({ replayGain: { trackGain: 0 } }))).toBe(0);
		expect(trackGain(track({ replayGain: { albumGain: -6.2 } }))).toBeNull();
		expect(trackGain(track())).toBeNull();
	});
});

describe('formatOf', () => {
	it('shows depth and rate where they mean something', () => {
		expect(formatOf(track({ bitDepth: 24, sampleRate: 96_000 }))).toBe('FLAC 24/96');
		expect(formatOf(track({ bitDepth: 16, sampleRate: 44_100 }))).toBe('FLAC 16/44.1');
		expect(formatOf(track({ format: 'MP3', sampleRate: 44_100 }))).toBe('MP3');
	});
});

describe('formatLine', () => {
	const hi = track({ bitDepth: 24, sampleRate: 96_000 });
	const cd = track({ bitDepth: 16, sampleRate: 44_100 });
	const mp3 = track({ format: 'MP3', genres: [], etag: '' });
	const ogg = track({ format: 'Ogg' });

	it('gives the one format an album has', () => {
		expect(formatLine([hi, hi])).toBe('FLAC 24/96');
	});

	it('falls back to the codecs when tracks differ', () => {
		expect(formatLine([hi, cd])).toBe('FLAC');
		expect(formatLine([mp3, hi])).toBe('FLAC · MP3');
	});

	it('keeps the first of many with a plus', () => {
		expect(formatLine([ogg, mp3, hi])).toBe('FLAC +');
	});
});

describe('albumFacts', () => {
	it('drops a missing year', () => {
		expect(albumFacts(album, album)).toBe('ALBUM · 2 TRACKS · 25 MIN');
		expect(albumFacts(album, { ...album, year: 1999 })).toBe('ALBUM · 1999 · 2 TRACKS · 25 MIN');
	});

	it('is singular for one track, and never says zero minutes', () => {
		const single = { ...album, tracks: [track()], durationMs: 12_000 };
		expect(albumFacts(single, single)).toBe('ALBUM · 1 TRACK · 1 MIN');
	});
});

describe('albumsLine', () => {
	it('counts albums and genres', () => {
		expect(albumsLine(927, 927, 105, null)).toBe('927 ALBUMS · 105 GENRES');
		expect(albumsLine(1, 1, 1, null)).toBe('1 ALBUM · 1 GENRE');
	});

	it('leaves out genres it has yet to count', () => {
		expect(albumsLine(60, 927, 0, null)).toBe('927 ALBUMS');
	});

	it('says what a genre leaves', () => {
		expect(albumsLine(83, 927, 105, 'Drum & Bass')).toBe('83 OF 927 ALBUMS · DRUM & BASS');
	});
});

describe('artistFacts', () => {
	const artist = { id: 'a', name: 'A', albumCount: 2, albums: [], appearsOn: [] };

	it('counts what an artist owns', () => {
		expect(artistFacts(artist)).toBe('ARTIST · 2 ALBUMS IN YOUR LIBRARY');
		expect(artistFacts({ ...artist, albumCount: 1 })).toBe('ARTIST · 1 ALBUM IN YOUR LIBRARY');
	});

	it('counts where a guest appears', () => {
		const entry = { id: 'b', title: 'B', artist: 'C', trackCount: 1, compilation: false };
		expect(artistFacts({ ...artist, albumCount: 0, appearsOn: [entry] })).toBe(
			'ARTIST · GUEST ON 1 ALBUM'
		);
	});
});

describe('artistCount', () => {
	it('says one or many', () => {
		expect(artistCount(1)).toBe('1 artist');
		expect(artistCount(1200)).toBe('1,200 artists');
	});
});

describe('the Library’s lines', () => {
	it('says where you are and how much there is', () => {
		expect(libraryLine('L', 912, 7586)).toBe('L · 912 ARTISTS · 7,586 SONGS');
	});

	it('counts what an artist has', () => {
		const albums = [{ tracks: [1, 2] }, { tracks: [3] }];
		expect(artistHoldings(albums)).toBe('2 ALBUMS · 3 SONGS');
		expect(artistHoldings([{ tracks: [1] }])).toBe('1 ALBUM · 1 SONG');
	});

	it('drops an album’s year when it has none', () => {
		expect(albumHoldings(2021, 10)).toBe('2021 · 10 songs');
		expect(albumHoldings(undefined, 1)).toBe('1 song');
	});
});

describe('credit on a shared album', () => {
	it('adds nothing when the song is by everyone the album names', () => {
		expect(credit({ artists: ['Ann', 'Bob'] }, 'Ann, Bob')).toBeUndefined();
	});

	it('still names a song by only some of them', () => {
		expect(credit({ artists: ['Ann'] }, 'Ann, Bob')).toBe('Ann');
	});
});
