import { describe, expect, it } from 'vitest';
import type { ImageCandidate } from './types.ts';
import {
	autoNote,
	captions,
	confirmWords,
	doneWords,
	failedWords,
	identityLine,
	pinnedWhen,
	shortMbid,
	sourceNotes
} from './words.ts';

const now = new Date('2026-11-27T15:00:00');

function candidate(over: Partial<ImageCandidate>): ImageCandidate {
	return {
		id: 'fanart:1',
		source: 'fanart',
		width: 1920,
		height: 1080,
		chosen: false,
		pinned: false,
		...over
	};
}

const text = (parts: { text: string }[]): string => parts.map((part) => part.text).join('');

describe('identityLine', () => {
	it('names the artist and the MBID it is asked by', () => {
		expect(identityLine('Low Tide Choir', '5b1c0d2e-aaaa-bbbb-cccc-0123456789e2', false)).toBe(
			'LOW TIDE CHOIR · BY MUSICBRAINZ ID 5B1C…E2'
		);
		expect(identityLine('Low Tide Choir', '5b1c0d2e-aaaa-bbbb-cccc-0123456789e2', true)).toBe(
			'LOW TIDE CHOIR · 5B1C…E2'
		);
		expect(identityLine('Ana', undefined, false)).toBe('ANA · NO MUSICBRAINZ ID');
	});

	it('leaves a short ID whole', () => {
		expect(shortMbid('abc')).toBe('abc');
	});
});

describe('captions', () => {
	it('says source and size, then the language or no words', () => {
		const [first, second] = captions(
			candidate({ width: 3840, height: 2160, lang: 'en' }),
			now,
			false
		);
		expect(text(first)).toBe('FANART.TV · 3840 × 2160');
		expect(text(second)).toBe('EN');
		expect(text(captions(candidate({}), now, false)[1])).toBe('NO WORDS');
	});

	it('puts the size on the second line on a phone', () => {
		const [first, second] = captions(candidate({}), now, true);
		expect(text(first)).toBe('FANART.TV');
		expect(text(second)).toBe('1920 × 1080');
	});

	it('says the chosen one is automatic, or who pinned it and when', () => {
		expect(captions(candidate({ chosen: true }), now, false)[1]).toEqual([
			{ text: 'CHOSEN · AUTOMATIC', fox: true }
		]);
		const pinned = candidate({
			chosen: true,
			pinned: true,
			pinnedBy: 'Simon',
			pinnedAt: '2026-11-27T09:00:00'
		});
		expect(text(captions(pinned, now, false)[1])).toBe('PINNED BY SIMON · TODAY');
		expect(
			text(captions({ ...pinned, pinnedBy: undefined } as ImageCandidate, now, false)[1])
		).toBe('PINNED · TODAY');
	});

	it('credits a Commons picture by its author, its licence a link', () => {
		const [first, second] = captions(
			candidate({
				source: 'wikimedia',
				credit: { author: 'K. Arnesen', license: 'CC BY-SA 4.0', licenseUrl: 'https://l' }
			}),
			now,
			false
		);
		expect(text(first)).toBe('COMMONS · K. ARNESEN');
		expect(second).toEqual([{ text: 'CC BY-SA 4.0 ›', href: 'https://l' }]);
	});

	it('says a logo with alpha is a transparent PNG, and an unknown size is left out', () => {
		const logo = candidate({
			width: 0,
			height: 0,
			preview: {
				hash: 'a'.repeat(64),
				accent: '#000000',
				ground: '#000000',
				width: 400,
				height: 155,
				alpha: true
			}
		});
		const [first, second] = captions(logo, now, false);
		expect(text(first)).toBe('FANART.TV');
		expect(text(second)).toBe('TRANSPARENT PNG');
		expect(captions(candidate({ source: 'upload' }), now, false)[1]).toEqual([]);
	});
});

describe('pinnedWhen', () => {
	it('says today, yesterday, else the day', () => {
		expect(pinnedWhen('2026-11-26T22:00:00', now)).toBe('YESTERDAY');
		expect(pinnedWhen('2026-11-02T10:00:00', now)).toBe('2 NOV');
		expect(pinnedWhen(undefined, now)).toBe('');
		expect(pinnedWhen('nonsense', now)).toBe('');
	});
});

describe('notes', () => {
	it('says what a pin means beside Automatic', () => {
		expect(autoNote(true)).toContain('AUTOMATIC UNPINS');
		expect(autoNote(false)).toBe('FILES FIRST, THEN FANART.TV, THEN THE REST');
	});

	it('says each source state once, held first and alone', () => {
		expect(
			sourceNotes({ held: true, sources: [{ source: 'fanart', state: 'held' }] })
		).toHaveLength(1);
		expect(
			sourceNotes({
				held: false,
				sources: [
					{ source: 'fanart', state: 'failed', words: 'fanart.tv answered 503' },
					{ source: 'wikimedia', state: 'unavailable' },
					{ source: 'fanart', state: 'unavailable' }
				]
			})
		).toEqual(['FANART.TV ANSWERED 503', 'NO MUSICBRAINZ ID · MATCH ITS ALBUMS TO FETCH ART']);
		expect(
			sourceNotes({ held: false, sources: [{ source: 'fanart', state: 'answered' }] })
		).toEqual([]);
	});
});

describe('confirm and toasts', () => {
	it('words Not this artist by the MBID', () => {
		const words = confirmWords('Low Tide Choir', '5b1c0d2e-aaaa-bbbb-cccc-0123456789e2');
		expect(words.kicker).toBe('NOT THIS ARTIST · LOW TIDE CHOIR');
		expect(words.body).toContain('the MusicBrainz ID 5b1c…e2');
	});

	it('names the artist in bold', () => {
		expect(doneWords('pinned', 'Low Tide Choir', 'background')).toEqual([
			{ text: 'Pinned ' },
			{ text: 'Low Tide Choir', bold: true },
			{ text: '’s background' }
		]);
		expect(text(doneWords('rejected', 'X', 'thumb'))).toBe(
			'X is held back: what the sources gave is gone'
		);
	});

	it('prefers the server’s words, else what the status means', () => {
		expect(failedWords(422, 'not a decodable image', 'x')).toBe('Not a decodable image');
		expect(failedWords(415, '', 'x')).toBe('That isn’t a JPEG, PNG or WebP');
		expect(failedWords(500, '', 'Couldn’t pin it')).toBe('Couldn’t pin it');
	});
});
