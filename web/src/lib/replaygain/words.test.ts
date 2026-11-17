import { describe, expect, it } from 'vitest';
import type { GainLeft, GainRun } from './types.ts';
import {
	albumChip,
	albumLine,
	biggestLine,
	confirmWords,
	dB,
	doneWords,
	formatsWords,
	headWords,
	leftHeading,
	levelledWords,
	menuNote,
	moreLeftLine,
	moreWords,
	navNote,
	nowLine,
	paceWords,
	problemWords,
	runRowWords,
	tagsLine,
	tookOutToast,
	undoAsk,
	waitingLine,
	whereLine,
	writeWords,
	wroteToast
} from './words.ts';

const run = (over: Partial<GainRun> = {}): GainRun => ({
	id: 'r',
	kind: 'run',
	state: 'done',
	startedAt: '2026-11-17T21:38:22Z',
	finishedAt: '2026-11-17T21:40:00Z',
	total: 1614,
	done: 1614,
	written: 1609,
	left: 5,
	failed: 0,
	albums: 90,
	albumsDone: 90,
	undoable: true,
	...over
});

const now = Date.parse('2026-11-17T22:00:00Z');

describe('the ready card', () => {
	it('says where, with the formats as a sentence', () => {
		expect(whereLine(90, ['FLAC', 'MP3'])).toBe('ON 90 ALBUMS · FLAC AND MP3');
		expect(whereLine(1, ['flac'])).toBe('ON 1 ALBUM · FLAC');
		expect(formatsWords(['FLAC', 'MP3', 'WAV'])).toBe('FLAC, MP3 AND WAV');
		expect(formatsWords([])).toBe('');
	});

	it('counts the albums past the chips', () => {
		expect(moreWords(90, 3)).toBe('+ 87 MORE');
		expect(moreWords(90, 4)).toBe('+ 86 MORE');
		expect(moreWords(3, 3)).toBe('');
	});

	it('puts the biggest on one line for a phone', () => {
		const album = (title: string, songs: number) => ({
			id: title,
			title,
			artist: '',
			formats: [],
			songs
		});
		expect(biggestLine([album('Rayman 3', 292), album('Persona5', 1110)])).toBe(
			'BIGGEST: RAYMAN 3 (292), PERSONA5 (1,110)'
		);
		expect(biggestLine([])).toBe('');
	});

	it('says the number, the button and what waits, in the singular too', () => {
		expect(levelledWords(1614)).toBe('songs are levelled only in Kitsune');
		expect(levelledWords(1)).toBe('song is levelled only in Kitsune');
		expect(writeWords(1614)).toBe('Write ReplayGain to 1,614 files');
		expect(writeWords(1)).toBe('Write ReplayGain to 1 file');
		expect(waitingLine(212, 0)).toBe('212 STILL BEING MEASURED · WRITE THEM ONCE THEY ARE');
		expect(waitingLine(1, 3)).toBe(
			'1 STILL BEING MEASURED · WRITE IT ONCE IT IS · 3 CAN’T BE MEASURED YET'
		);
		expect(waitingLine(0, 0)).toBe('');
	});
});

describe('a run', () => {
	it('heads a run and an undo', () => {
		expect(headWords({ kind: 'run', done: 612, total: 1614 })).toBe('Writing 612 of 1,614');
		expect(headWords({ kind: 'undo', done: 12, total: 1609 })).toBe('Taking out 12 of 1,609');
	});

	it('works out the pace and the time left, rounded as said', () => {
		expect(paceWords(0, 1614, 1000)).toBe('STARTING');
		// 612 in 36.72 s: 1,000 a minute, 1,002 left.
		expect(paceWords(612, 1614, 36_720)).toBe('ABOUT 1 MIN LEFT · ABOUT 1,000 FILES A MINUTE');
		expect(paceWords(100, 1614, 60_000)).toBe('ABOUT 15 MIN LEFT · ABOUT 100 FILES A MINUTE');
		expect(paceWords(1600, 1614, 60_000)).toBe('UNDER A MINUTE LEFT · ABOUT 1,600 FILES A MINUTE');
		expect(paceWords(10, 10, 60_000)).toBe('ALMOST DONE · ABOUT 10 FILES A MINUTE');
		expect(paceWords(1, 100, 120_000)).toBe('ABOUT 198 MIN LEFT · ABOUT 1 FILE A MINUTE');
	});

	it('says the album in hand and what Stop does', () => {
		expect(nowLine('Rayman 2 - The Complete Soundtrack')).toBe(
			'NOW · RAYMAN 2 - THE COMPLETE SOUNDTRACK · STOP FINISHES THIS ALBUM, THEN STOPS · MUSIC KEEPS PLAYING'
		);
		expect(nowLine('')).toBe('STOP FINISHES THIS ALBUM, THEN STOPS · MUSIC KEEPS PLAYING');
	});

	it('gives each album its line and chip', () => {
		expect(
			albumLine({
				id: 'a',
				title: 'Calling All Dawns',
				artist: 'Christopher Tin',
				year: 2009,
				formats: ['MP3'],
				songs: 12
			})
		).toBe('CHRISTOPHER TIN · 2009 · MP3 · 12 SONGS');
		expect(albumLine({ id: 'a', title: 'x', artist: 'Nobody', formats: [], songs: 1 })).toBe(
			'NOBODY · 1 SONG'
		);
		expect(albumChip('written')).toBe('WRITTEN ✓');
		expect(albumChip('writing')).toBe('WRITING');
		expect(albumChip('waiting')).toBe('WAITING');
		expect(albumChip('writing', true)).toBe('TAKING OUT');
		expect(albumChip('left')).toBe('LEFT AS IT WAS');
	});

	it('notes a run in the nav', () => {
		expect(navNote(612, 1614)).toBe('REPLAYGAIN · 37%');
		expect(navNote(0, 0)).toBe('REPLAYGAIN · 0%');
	});
});

describe('done', () => {
	it('says what a finished run did, and how long it took', () => {
		const words = doneWords(run(), now);
		expect(words.kicker).toMatch(/^FINISHED · TODAY · \d\d:\d\d · TOOK 1 MIN 38 S$/);
		expect(words.title).toBe('Wrote ReplayGain to 1,609 songs');
		expect(words.body).toContain('every other player levels them now too');
		expect(doneWords(run({ written: 1 }), now).title).toBe('Wrote ReplayGain to 1 song');
	});

	it('says a stopped run stopped at an album’s end', () => {
		const words = doneWords(run({ state: 'stopped' }), now);
		expect(words.kicker).toMatch(/^STOPPED · TODAY · \d\d:\d\d · AT AN ALBUM’S END$/);
		expect(words.body).toContain('Write again to carry on.');
	});

	it('says an undo took it out, and a run undone since', () => {
		const undo = doneWords(run({ kind: 'undo', written: 12 }), now);
		expect(undo.kicker).toMatch(/^UNDONE · /);
		expect(undo.title).toBe('Took ReplayGain out of 12 files');
		expect(doneWords(run({ state: 'undone', undoable: false }), now).kicker).toMatch(
			/^UNDONE · TODAY/
		);
	});

	it('words each reason a song was left, and the ones past the first few', () => {
		const left = (reason: GainLeft['reason']): GainLeft => ({
			track: reason,
			title: reason,
			path: '',
			root: 0,
			reason
		});
		expect(leftHeading(5)).toBe('LEFT AS THEY WERE · 5');
		expect(leftHeading(1)).toBe('LEFT AS IT WAS · 1');
		expect(moreLeftLine([left('changed'), left('changed')])).toBe(
			'+ 2 MORE · CHANGED SINCE THE SCAN'
		);
		expect(moreLeftLine([left('trashed'), left('stale'), left('stale')], 10)).toBe(
			'+ 13 MORE · MEASURE OUT OF DATE · IN THE TRASH'
		);
		expect(moreLeftLine([])).toBe('');
	});

	it('lists runs with Undo while they can be', () => {
		expect(runRowWords(run())).toEqual({ what: 'Wrote 1,609 songs, 5 left', state: 'UNDO ›' });
		expect(runRowWords(run({ left: 0, undoable: false }))).toEqual({
			what: 'Wrote 1,609 songs',
			state: 'DONE'
		});
		expect(runRowWords(run({ state: 'undone', undoable: false })).state).toBe('UNDONE');
		expect(runRowWords(run({ state: 'stopped', undoable: false })).state).toBe('STOPPED');
		expect(runRowWords(run({ state: 'running' })).state).toBe('RUNNING');
		expect(runRowWords(run({ kind: 'undo', written: 1, undoable: false })).what).toBe(
			'Took it out of 1 file'
		);
		expect(undoAsk(run())).toBe('Take ReplayGain out of 1,609 files?');
	});
});

describe('an album', () => {
	it('notes the ⋯ item', () => {
		expect(menuNote(12)).toBe('12 SONGS MEASURED · WRITES THEM TO THE FILES');
		expect(menuNote(1)).toBe('1 SONG MEASURED · WRITES IT TO THE FILE');
	});

	it('says what the confirm writes', () => {
		expect(confirmWords(12, ['FLAC'], -10.31, 1.0712)).toEqual({
			kicker: 'WRITE REPLAYGAIN · 12 FILES · FLAC',
			title: 'Write Kitsune’s measure into these 12 files?',
			album: '−10.31 DB · PEAK 1.071',
			button: 'Write 12 files'
		});
		const one = confirmWords(1, [], undefined, undefined);
		expect(one.kicker).toBe('WRITE REPLAYGAIN · 1 FILE');
		expect(one.title).toBe('Write Kitsune’s measure into this file?');
		expect(one.album).toBe('');
		expect(dB(1.2)).toBe('+1.20 DB');
	});

	it('words the toast', () => {
		expect(wroteToast(12, 0)).toBe('Wrote ReplayGain to 12 files of');
		expect(wroteToast(0, 12)).toBe('Its files changed since the scan: nothing was written to');
		expect(tookOutToast(1)).toBe('Took ReplayGain out of 1 file of');
		expect(problemWords('a run holds the library', 'x')).toBe('A run holds the library.');
		expect(problemWords('', 'Try again.')).toBe('Try again.');
	});
});

describe('what it writes', () => {
	it('names each format’s tags, formats sharing a scheme together', () => {
		expect(tagsLine(['FLAC', 'MP3'])).toBe('FLAC: VORBIS COMMENTS · MP3: ID3 TXXX');
		expect(tagsLine(['mp3', 'WAV', 'FLAC'])).toBe('MP3 AND WAV: ID3 TXXX · FLAC: VORBIS COMMENTS');
		expect(tagsLine([])).toBe('FLAC: VORBIS COMMENTS · MP3: ID3 TXXX');
	});
});
