import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import {
	byteLength,
	extensionOf,
	mostPathBytes,
	mostSegmentBytes,
	parse,
	pathOf,
	safe,
	TemplateError,
	tryParse,
	type Song
} from './template.ts';

/** The cases the server's `internal/organize` is held to as well. */
interface Fixtures {
	parse: { template: string; error?: { at: number; reason: string } }[];
	paths: {
		name: string;
		template: string;
		song: Partial<Song>;
		ext: string;
		path?: string;
		error?: string;
	}[];
	safe: { in: string; out: string }[];
}

const fixtures = JSON.parse(
	readFileSync(
		new URL('../../../../internal/organize/testdata/templates.json', import.meta.url),
		'utf8'
	)
) as Fixtures;

const blank: Song = {
	title: '',
	album: '',
	albumArtist: '',
	artists: [],
	genre: '',
	discName: '',
	format: '',
	year: 0,
	originalYear: 0,
	disc: 0,
	discs: 0,
	track: 0,
	discSongs: 0,
	compilation: false
};

describe('the shared fixtures', () => {
	it('has every kind of case', () => {
		expect(fixtures.parse.length).toBeGreaterThan(10);
		expect(fixtures.paths.length).toBeGreaterThan(10);
		expect(fixtures.safe.length).toBeGreaterThan(10);
	});

	it.each(fixtures.parse)('parses $template', ({ template, error }) => {
		const got = tryParse(template);
		if (error === undefined) {
			expect(got).not.toBeInstanceOf(TemplateError);
			return;
		}
		expect(got).toBeInstanceOf(TemplateError);
		const { at, reason } = got as TemplateError;
		expect({ at, reason }).toEqual(error);
	});

	it.each(fixtures.paths)('files $name', ({ template, song, ext, path, error }) => {
		const got = pathOf(parse(template), { ...blank, ...song }, ext);
		expect(got).toBe(error ?? path);
	});

	it.each(fixtures.safe)('makes $in safe', ({ in: input, out }) => {
		expect(safe(input)).toBe(out);
		expect(safe(out)).toBe(out);
	});
});

describe('safe', () => {
	it('cuts a long name at a character boundary, keeping its extension', () => {
		const got = safe('あ'.repeat(59) + 'ab' + 'い'.repeat(20) + '.flac');
		expect(byteLength(got)).toBeLessThanOrEqual(mostSegmentBytes);
		expect(got).toBe('あ'.repeat(58) + '.flac');
	});

	it('is idempotent on awkward names', () => {
		for (const name of ['', '.', '..', 'CON.', 'a/b', '  .x. ', 'é́', '@ .', 'COM1 .flac']) {
			const once = safe(name);
			expect(safe(once)).toBe(once);
		}
	});
});

describe('pathOf', () => {
	it('cuts the title first, then the album, past the path limit', () => {
		const t = parse('<Album Artist>/<Genre>/<Disc Name>/<Album>/<Track> - <Title>');
		const song: Song = {
			...blank,
			albumArtist: 'a'.repeat(170),
			genre: 'g'.repeat(170),
			discName: 'd'.repeat(170),
			album: 'b'.repeat(170),
			title: 't'.repeat(400),
			track: 1,
			discs: 1,
			discSongs: 1
		};
		const got = pathOf(t, song, '.flac');
		expect(byteLength(got)).toBeLessThanOrEqual(mostPathBytes);
		expect(got.split('/')[3]).toBe('b'.repeat(170));
	});

	it('takes the extension from the last name', () => {
		expect(extensionOf('Artist/Vol. 2/01 - A.flac')).toBe('.flac');
		expect(extensionOf('Artist/x')).toBe('');
	});
});
