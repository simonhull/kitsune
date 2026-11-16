import { describe, expect, it } from 'vitest';
import type { MissingLine } from '#lib/library/source.ts';
import {
	changedAgo,
	countWithFiles,
	downloadLine,
	existsWords,
	exportFileName,
	exportHead,
	extinfWords,
	fileCardLine,
	fileHeadLine,
	filePhoneHeadLine,
	folderLabel,
	hintFor,
	missingHeading,
	missingLead,
	missingText,
	notFoundPill,
	readWords,
	reasonWords,
	saveAsWords,
	savedWords,
	underRoot
} from './fileWords.ts';

const day = 86_400_000;
const now = Date.parse('2026-11-16T12:00:00Z');
const minutes79 = 79 * 60_000;

describe('a file card and head', () => {
	it('says its size, and what it lost in a part of its own', () => {
		expect(fileCardLine({ songCount: 19, durationMs: minutes79, file: fileOf(3) })).toEqual([
			'19 SONGS · 1 H 19 MIN · ',
			'3 NOT FOUND'
		]);
		expect(fileCardLine({ songCount: 12, durationMs: 58 * 60_000, file: fileOf(0) })).toEqual([
			'12 SONGS · 58 MIN',
			''
		]);
	});

	it('heads its page as a file everyone sees', () => {
		expect(fileHeadLine({ songCount: 19, albumCount: 7, durationMs: minutes79 })).toBe(
			'FILE PLAYLIST · 19 SONGS · 7 ALBUMS · 1 H 19 MIN · EVERYONE HERE'
		);
		expect(filePhoneHeadLine({ songCount: 19, durationMs: minutes79 })).toBe(
			'FILE · 19 SONGS · 1 H 19 · EVERYONE'
		);
	});

	it('says when it was read, and how many lines found nothing', () => {
		expect(readWords(new Date(now - 2 * day).toISOString(), now)).toBe('READ 2 DAYS AGO');
		expect(notFoundPill(3, false)).toBe('3 SONGS NOT FOUND');
		expect(notFoundPill(1, false)).toBe('1 SONG NOT FOUND');
		expect(notFoundPill(3, true)).toBe('3 NOT FOUND');
		expect(missingHeading(3)).toBe('3 lines found no song');
		expect(missingLead(19, false)).toBe(
			"THE OTHER 19 PLAY IN THE FILE'S ORDER · FIX THE FILE AND KITSUNE READS IT AGAIN"
		);
		expect(missingLead(19, true)).toBe('FIX THE FILE AND KITSUNE READS IT AGAIN');
	});

	it('counts the files on the Playlists page in place of YOURS ALONE', () => {
		expect(countWithFiles(4, 1, 2)).toBe('4 PLAYLISTS · 1 SMART · 2 FROM FILES');
		expect(countWithFiles(3, 1, 0)).toBe('3 PLAYLISTS · 1 SMART · YOURS ALONE');
	});
});

describe('the lines that found no song', () => {
	const golden: MissingLine = {
		line: 9,
		text: 'Falling Up/2009 - Fangs!/04 Golden Arrows.mp3',
		title: 'Falling Up - Golden Arrows',
		seconds: 246,
		reason: 'noSong',
		otherExtension: '04 Golden Arrows.flac'
	};

	it('words each reason', () => {
		expect(reasonWords('noSong')).toBe('NO SONG AT THIS PATH');
		expect(reasonWords('outside')).toBe('OUTSIDE THE LIBRARY');
		expect(reasonWords('notFile')).toBe('NOT A FILE');
		expect(reasonWords('trashed')).toBe('IN THE TRASH');
		expect(reasonWords('pastLimit')).toBe('PAST THE LIMIT');
	});

	it('names the song with another extension, never matching it', () => {
		expect(hintFor(golden, false)).toBe(
			'Fangs! has 04 Golden Arrows.flac: the file names the .mp3.'
		);
		expect(hintFor(golden, true)).toBe('Fangs! has 04 Golden Arrows.flac.');
		expect(extinfWords(golden)).toBe('Falling Up - Golden Arrows · 4:06');
	});

	it('says a web address is one, and why the others found nothing', () => {
		const stream: MissingLine = { line: 15, text: 'https://…', reason: 'notFile' };
		expect(missingText(stream)).toBe('https://… (a web address)');
		expect(hintFor(stream, false)).toBe('Kitsune plays songs in its library, not streams.');
		expect(hintFor({ line: 20, text: '…/a/b.mp3', reason: 'outside' }, false)).toBe(
			'An absolute path in no library folder. Only its end is shown.'
		);
		expect(hintFor({ line: 3, text: 'a/b.flac', reason: 'trashed' }, false)).toBe(
			'Restore it and it’s back here.'
		);
	});
});

describe('Export', () => {
	it('names the file by the server’s rule', () => {
		expect(exportFileName('Falling Up')).toBe('Falling Up.m3u8');
		expect(exportFileName('../AC/DC\\live')).toBe('ACDClive.m3u8');
		expect(exportFileName('.hidden')).toBe('hidden.m3u8');
		expect(exportFileName('tab\there')).toBe('tabhere.m3u8');
		expect(exportFileName('  /  ')).toBe('playlist.m3u8');
		const long = exportFileName('é'.repeat(100));
		expect(new TextEncoder().encode(long.replace(/\.m3u8$/, '')).length).toBe(120);
	});

	it('says what the download holds', () => {
		expect(exportHead(151)).toBe('EXPORT · 151 SONGS AS .M3U8');
		expect(downloadLine('Falling Up', 151)).toBe(
			'Falling Up.m3u8 · 151 LINES · #EXTINF WITH ARTIST - TITLE'
		);
	});

	it('shows a folder by its library folder’s own name, and finds one in the roots', () => {
		expect(folderLabel('/music', 'Playlists')).toBe('music / Playlists');
		expect(folderLabel('/srv/music/', '')).toBe('music');
		expect(underRoot('/music/Playlists/Old', ['/music'])).toEqual({
			root: '/music',
			folder: 'Playlists/Old'
		});
		expect(underRoot('/music', ['/music'])).toEqual({ root: '/music', folder: '' });
		expect(underRoot('/musical', ['/music'])).toBeNull();
	});

	it('asks before replacing, offering the next free name', () => {
		const exists = {
			name: 'Falling Up.m3u8',
			freeName: 'Falling Up 2.m3u8',
			modifiedAt: new Date(now - 21 * day).toISOString(),
			playlistName: 'Falling Up',
			songCount: 148
		};
		expect(
			existsWords(exists, now)
				.map(({ text }) => text)
				.join('')
		).toBe(
			'Kitsune reads it as Falling Up, a file playlist of 148 songs, last changed 3 weeks ago. Replacing it changes that playlist for everyone.'
		);
		expect(saveAsWords(exists.freeName)).toBe('Save as Falling Up 2.m3u8');
		expect(changedAgo(new Date(now - day).toISOString(), now)).toBe('yesterday');
	});

	it('says where it saved, and what it left out', () => {
		expect(
			savedWords(
				{ name: 'Falling Up.m3u8', folder: 'Playlists', songCount: 151, leftOut: 0 },
				'/music'
			)
		).toEqual({
			head: 'Saved Falling Up.m3u8',
			body: 'In music / Playlists. It shows under From files in a moment, when the library reads it.'
		});
		expect(
			savedWords({ name: 'a.m3u8', folder: '', songCount: 3, leftOut: 2 }, '/music').body
		).toContain('2 songs are in another library folder and were left out.');
	});
});

function fileOf(notFound: number) {
	return {
		path: 'Playlists/x.m3u8',
		format: 'm3u8' as const,
		notFound,
		pastLimit: 0,
		readAt: '2026-11-14T12:00:00Z',
		modifiedAt: '2026-11-14T12:00:00Z'
	};
}
