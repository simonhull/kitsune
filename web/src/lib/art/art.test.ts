import { describe, expect, it } from 'vitest';
import type { ArtistBio, ExtraImage, LibraryIndex, LibraryTrack } from '#lib/library/source.ts';
import {
	artworkStrip,
	bioCredit,
	bioRest,
	bioTag,
	discArtFor,
	drawPictures,
	fallbackAlbum,
	imageCredits,
	initialColors,
	initialOf,
	phoneStagePicture,
	pictureRung,
	pictureUrl,
	stripCount
} from './art.ts';

function image(source: ExtraImage['source'], credit?: ExtraImage['credit']): ExtraImage {
	return {
		hash: 'a'.repeat(64),
		accent: '#ffffff',
		ground: '#000000',
		width: 1000,
		height: 1000,
		alpha: false,
		source,
		...(credit === undefined ? {} : { credit })
	};
}

describe('pictureRung', () => {
	it('takes the smallest rendition of the kind’s ladder that covers the box', () => {
		expect(pictureRung('thumb', 80)).toBe(128);
		expect(pictureRung('thumb', 140)).toBe(512);
		expect(pictureRung('background', 1440)).toBe(1920);
		expect(pictureRung('background', 2880)).toBe(3840);
		expect(pictureRung('logo', 520)).toBe(800);
		expect(pictureRung('disc', 340)).toBe(400);
	});

	it('stops at the top of the ladder', () => {
		expect(pictureRung('thumb', 4000)).toBe(1024);
		expect(pictureRung('logo', 4000)).toBe(800);
	});
});

describe('pictureUrl', () => {
	it('asks the server, unless the fixtures drew the picture', () => {
		expect(pictureUrl('b'.repeat(64), 512)).toContain(`/artwork/${'b'.repeat(64)}?size=512`);
		drawPictures([['c'.repeat(64), 'data:image/svg+xml,drawn']]);
		expect(pictureUrl('c'.repeat(64), 512)).toBe('data:image/svg+xml,drawn');
	});
});

describe('initialOf', () => {
	it('takes the first letter or digit, as a capital', () => {
		expect(initialOf('Gully Saints')).toBe('G');
		expect(initialOf('  ëlan')).toBe('Ë');
		expect(initialOf('!!!')).toBe('?');
		expect(initialOf('(Sandy) Alex G')).toBe('S');
		expect(initialOf('2 Many DJs')).toBe('2');
	});
});

describe('initialColors', () => {
	it('gives an artist the same colours every time, never a grey', () => {
		const one = initialColors('artist-gully-saints');
		expect(initialColors('artist-gully-saints')).toEqual(one);
		expect(one.ground).toMatch(/^#[0-9A-F]{6}$/iu);
		expect(one.ink).not.toBe(one.ground);
	});
});

describe('imageCredits', () => {
	it('names one source once for every picture it gave, with its page', () => {
		const fanart = image('fanart', { pageUrl: 'https://fanart.tv/x' });
		expect(imageCredits({ thumb: fanart, background: fanart })).toEqual([
			[{ text: 'PHOTO AND BACKDROP' }, { text: 'FANART.TV ›', href: 'https://fanart.tv/x' }]
		]);
	});

	it('credits a Commons portrait with its author and license as a link', () => {
		const commons = image('wikimedia', {
			author: 'J. Rowe',
			license: 'CC BY-SA 4.0',
			licenseUrl: 'https://creativecommons.org/licenses/by-sa/4.0/',
			pageUrl: 'https://commons.wikimedia.org/x'
		});
		expect(imageCredits({ thumb: commons })).toEqual([
			[
				{ text: 'PHOTO' },
				{ text: 'J. ROWE' },
				{ text: 'CC BY-SA 4.0', href: 'https://creativecommons.org/licenses/by-sa/4.0/' },
				{ text: 'COMMONS ›', href: 'https://commons.wikimedia.org/x' }
			]
		]);
	});

	it('says nothing for the library’s own files', () => {
		expect(imageCredits({ thumb: image('folder'), background: image('folder') })).toEqual([]);
	});

	it('still credits a pinned picture by where its credit says it came from', () => {
		const pinned = image('pinned', { pageUrl: 'https://fanart.tv/x' });
		expect(
			imageCredits({
				background: pinned,
				thumb: image('fanart', { pageUrl: 'https://fanart.tv/x' })
			})
		).toEqual([
			[{ text: 'PHOTO AND BACKDROP' }, { text: 'FANART.TV ›', href: 'https://fanart.tv/x' }]
		]);
		const commons = image('pinned', {
			author: 'J. Rowe',
			license: 'CC BY-SA 4.0',
			pageUrl: 'https://commons.wikimedia.org/x'
		});
		expect(imageCredits({ thumb: commons })[0]?.at(-1)).toEqual({
			text: 'COMMONS ›',
			href: 'https://commons.wikimedia.org/x'
		});
		// An upload asks nothing to be said.
		expect(imageCredits({ thumb: image('pinned') })).toEqual([]);
	});

	it('lists three kinds with commas', () => {
		const fanart = image('fanart');
		expect(imageCredits({ thumb: fanart, background: fanart, logo: fanart })[0]?.[0]).toEqual({
			text: 'PHOTO, BACKDROP AND LOGO'
		});
	});
});

const wikipedia: ArtistBio = {
	summary: 'First.',
	body: 'First.\n\nSecond.\n\n\nThird.',
	source: 'wikipedia',
	url: 'https://en.wikipedia.org/wiki/X',
	license: 'CC BY-SA 4.0',
	licenseUrl: 'https://creativecommons.org/licenses/by-sa/4.0/'
};

describe('the bio', () => {
	it('wears a paper tag naming where it came from', () => {
		expect(bioTag('nfo')).toBe('YOUR FILES');
		expect(bioTag('wikipedia')).toBe('WIKIPEDIA');
		expect(bioTag('lastfm')).toBe('LAST.FM');
	});

	it('credits the source, its license as a link, the shortening and the article', () => {
		expect(bioCredit(wikipedia)).toEqual([
			{ text: 'FROM WIKIPEDIA' },
			{ text: 'CC BY-SA 4.0', href: 'https://creativecommons.org/licenses/by-sa/4.0/' },
			{ text: 'SHORTENED FROM THE ORIGINAL' },
			{ text: 'READ THE ARTICLE ›', href: 'https://en.wikipedia.org/wiki/X' }
		]);
		expect(bioCredit({ ...wikipedia, source: 'lastfm', license: 'CC BY-SA' }).at(-1)).toEqual({
			text: 'READ ON LAST.FM ›',
			href: 'https://en.wikipedia.org/wiki/X'
		});
	});

	it('says an artist.nfo is the library’s own, and whole', () => {
		expect(bioCredit({ summary: 'A', body: 'A', source: 'nfo' })).toEqual([
			{ text: 'FROM ARTIST.NFO IN YOUR LIBRARY' }
		]);
	});

	it('keeps for More what follows the summary', () => {
		expect(bioRest(wikipedia)).toEqual(['Second.', 'Third.']);
		expect(bioRest({ ...wikipedia, body: 'First.' })).toEqual([]);
	});
});

describe('album extras', () => {
	const disc = image('fanart');
	const two = image('folder');

	it('finds the disc a song is on, else every disc’s', () => {
		const extras = {
			discs: [
				{ disc: 0, image: disc },
				{ disc: 2, image: two }
			]
		};
		expect(discArtFor({ extras }, 2)?.image).toBe(two);
		expect(discArtFor({ extras }, 1)?.image).toBe(disc);
		expect(discArtFor({ extras }, undefined)?.image).toBe(disc);
		expect(discArtFor({}, 1)).toBeUndefined();
	});

	it('lays out the strip: front, back, each disc, with where each came from', () => {
		const strip = artworkStrip({
			extras: {
				discs: [
					{ disc: 1, image: disc },
					{ disc: 2, image: disc }
				],
				back: two,
				coverSource: 'files'
			},
			artwork: { hash: 'd'.repeat(64), accent: '#000000', ground: '#000000', width: 1, height: 1 }
		});
		expect(strip.map(({ label, from }) => `${label}:${from}`)).toEqual([
			'FRONT:IN THE FILES',
			'BACK:IN THE FILES',
			'DISC 1:FANART.TV',
			'DISC 2:FANART.TV'
		]);
		expect(stripCount(strip)).toBe('4 PICTURES · 2 DISCS');
	});

	it('has no strip with nothing beside the cover', () => {
		expect(artworkStrip({ extras: { discs: [] } })).toEqual([]);
		expect(artworkStrip({})).toEqual([]);
	});
});

describe('the stage', () => {
	it('stands on the thumb on a phone, the backdrop where there is none', () => {
		const thumb = image('fanart');
		const background = image('folder');
		expect(phoneStagePicture({ thumb, background })).toBe(thumb);
		expect(phoneStagePicture({ background })).toBe(background);
		expect(phoneStagePicture({})).toBeUndefined();
	});

	function track(id: string, plays: number): LibraryTrack {
		return { id, title: id, artists: [], durationMs: 1, plays } as LibraryTrack;
	}

	const index = {
		artists: [
			{
				id: 'x',
				name: 'X',
				sortName: 'X',
				albums: [
					{ id: 'new', tracks: [track('a', 1)] },
					{ id: 'old', tracks: [track('b', 3), track('c', 4)] },
					{ id: 'guest', tracks: [track('d', 90)] }
				]
			}
		]
	} as unknown as LibraryIndex;

	it('falls back on the album of theirs they play most, never one they only appear on', () => {
		expect(fallbackAlbum(index, ['new', 'old'])).toBe('old');
	});

	it('takes the newest when nothing has been played, and nothing with no albums', () => {
		const cold = {
			artists: [
				{
					id: 'x',
					name: 'X',
					sortName: 'X',
					albums: [
						{ id: 'new', tracks: [track('a', 0)] },
						{ id: 'old', tracks: [track('b', 0)] }
					]
				}
			]
		} as unknown as LibraryIndex;
		expect(fallbackAlbum(cold, ['new', 'old'])).toBe('new');
		expect(fallbackAlbum(index, [])).toBeUndefined();
		expect(fallbackAlbum(undefined, ['new'])).toBeUndefined();
	});
});
