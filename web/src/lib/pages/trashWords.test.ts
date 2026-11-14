import { describe, expect, it } from 'vitest';
import {
	daysLeft,
	dangerDays,
	groupLine,
	leftWords,
	restoreLabel,
	summaryWords,
	toastWords,
	whenWords
} from './trashWords.ts';

const dayMs = 86_400_000;
const now = new Date(2026, 10, 14, 21, 45).getTime();
const purgeIn = (days: number): string => new Date(now + days * dayMs).toISOString();

describe('toastWords', () => {
	it('names one song in bold, then says where it went', () => {
		expect(toastWords({ kind: 'song', title: 'Papercut' })).toEqual({
			name: 'Papercut',
			rest: ' moved to Trash'
		});
	});

	it('names an album and counts its songs', () => {
		expect(toastWords({ kind: 'album', title: 'Falling Up', songs: 11 })).toEqual({
			name: 'Falling Up',
			rest: ' · 11 songs moved to Trash'
		});
		expect(toastWords({ kind: 'album', title: 'Single', songs: 1 }).rest).toBe(
			' · 1 song moved to Trash'
		);
	});

	it('counts songs picked, with nothing in bold', () => {
		expect(toastWords({ kind: 'songs', songs: 3 })).toEqual({
			name: null,
			rest: '3 songs moved to Trash'
		});
		expect(toastWords({ kind: 'songs', songs: 1200 }).rest).toBe('1,200 songs moved to Trash');
	});
});

describe('leftWords', () => {
	it('counts the days left while there are more than five', () => {
		expect(leftWords(purgeIn(30), now)).toEqual({ text: '30 DAYS LEFT', danger: false });
		expect(leftWords(purgeIn(27), now)).toEqual({ text: '27 DAYS LEFT', danger: false });
		expect(leftWords(purgeIn(dangerDays + 1), now).danger).toBe(false);
	});

	it('says when it goes, in danger, at five days and under', () => {
		expect(leftWords(purgeIn(5), now)).toEqual({ text: 'GONE IN 5 DAYS', danger: true });
		expect(leftWords(purgeIn(4), now)).toEqual({ text: 'GONE IN 4 DAYS', danger: true });
		expect(leftWords(purgeIn(2), now)).toEqual({ text: 'GONE IN 2 DAYS', danger: true });
	});

	it('says tomorrow for the last day, and today once its time has come', () => {
		expect(leftWords(purgeIn(1), now)).toEqual({ text: 'GONE TOMORROW', danger: true });
		expect(leftWords(purgeIn(0.2), now)).toEqual({ text: 'GONE TOMORROW', danger: true });
		expect(leftWords(purgeIn(0), now)).toEqual({ text: 'GONE TODAY', danger: true });
		expect(leftWords(purgeIn(-1), now)).toEqual({ text: 'GONE TODAY', danger: true });
	});

	it('counts part of a day as a day', () => {
		expect(daysLeft(purgeIn(29.99), now)).toBe(30);
		expect(daysLeft(purgeIn(-3), now)).toBe(0);
	});
});

describe('whenWords', () => {
	it('gives the time and who for today and yesterday', () => {
		const today = new Date(2026, 10, 14, 21, 40).toISOString();
		const yesterday = new Date(2026, 10, 13, 9, 12).toISOString();
		expect(whenWords(today, 'Simon', now)).toBe('TODAY · 21:40 · SIMON');
		expect(whenWords(yesterday, 'Simon', now)).toBe('YESTERDAY · 09:12 · SIMON');
	});

	it('counts days before that, by the calendar', () => {
		const late = new Date(2026, 10, 11, 23, 59).toISOString();
		expect(whenWords(late, 'simon', now)).toBe('3 DAYS AGO · SIMON');
	});

	it('says when only, for someone whose account is gone', () => {
		const late = new Date(2026, 9, 19, 12, 0).toISOString();
		expect(whenWords(late, undefined, now)).toBe('26 DAYS AGO');
	});
});

describe('summaryWords', () => {
	it('counts the songs and their size, and says how long each stays', () => {
		const trash = { count: 6, bytes: 186_000_000, retentionDays: 30 };
		expect(summaryWords(trash, false)).toBe(
			'6 SONGS · 186 MB · EACH ONE GOES FOR GOOD 30 DAYS AFTER IT CAME'
		);
		expect(summaryWords(trash, true)).toBe('6 SONGS · 186 MB · 30 DAYS EACH');
	});

	it('says one song, and empty', () => {
		expect(summaryWords({ count: 1, bytes: 31_000_000, retentionDays: 30 }, true)).toBe(
			'1 SONG · 31 MB · 30 DAYS EACH'
		);
		expect(summaryWords({ count: 0, bytes: 0, retentionDays: 30 }, false)).toBe('EMPTY');
	});
});

describe('an album in Trash', () => {
	it('says who it is by and how many songs, and restores them by count', () => {
		expect(groupLine('Falling Up', 3)).toBe('FALLING UP · 3 SONGS');
		expect(groupLine('Linkin Park', 1)).toBe('LINKIN PARK · 1 SONG');
		expect(restoreLabel(1)).toBe('Restore');
		expect(restoreLabel(3)).toBe('Restore 3');
	});
});
