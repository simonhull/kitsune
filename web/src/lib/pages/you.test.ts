import { describe, expect, it } from 'vitest';
import { lastfmStatusOf } from '#lib/library/fixtures.ts';
import {
	ago,
	cardOf,
	countsLine,
	lastSentLine,
	playsWaiting,
	resultLine,
	resultOf,
	retryLine,
	roleLine
} from './you';

const now = Date.parse('2026-11-07T12:00:00Z');

describe('the You page', () => {
	it('draws each state the boards did', () => {
		for (const state of ['unset', 'none', 'connected', 'waiting', 'reconnect'] as const) {
			expect(cardOf(lastfmStatusOf(state))).toBe(state);
		}
	});

	it('says nothing is set up before it says anything about a connection', () => {
		expect(cardOf({ ...lastfmStatusOf('reconnect'), configured: false })).toBe('unset');
	});

	it('counts as board A does', () => {
		expect(countsLine({ plays: 3412, hearts: 64, rated: 211 })).toBe(
			'3,412 PLAYS · 64 HEARTS · 211 SONGS RATED'
		);
		expect(countsLine({ plays: 1, hearts: 1, rated: 1 })).toBe('1 PLAY · 1 HEART · 1 SONG RATED');
	});

	it('names the role', () => {
		expect(roleLine('admin')).toBe('YOU · ADMINISTRATOR');
		expect(roleLine('user')).toBe('YOU · LISTENER');
	});

	it('says how long ago', () => {
		expect(ago(now - 20_000, now)).toBe('just now');
		expect(ago(now - 2 * 60_000, now)).toBe('2 minutes ago');
		expect(ago(now - 60_000, now)).toBe('1 minute ago');
		expect(ago(now - 3 * 3_600_000, now)).toBe('3 hours ago');
		expect(ago(now - 2 * 86_400_000, now)).toBe('2 days ago');
		expect(lastSentLine(undefined, now)).toBe('nothing sent yet');
		expect(lastSentLine(new Date(now - 120_000).toISOString(), now)).toBe(
			'last scrobble 2 minutes ago'
		);
	});

	it('says when the next try is', () => {
		expect(retryLine(new Date(now + 3.5 * 60_000).toISOString(), now)).toBe(
			'TRYING AGAIN IN 4 MIN'
		);
		expect(retryLine(new Date(now - 1).toISOString(), now)).toBe('TRYING AGAIN NOW');
		expect(retryLine(undefined, now)).toBe('TRYING AGAIN SOON');
		expect(playsWaiting(1)).toBe('1 play');
		expect(playsWaiting(12)).toBe('12 plays');
	});

	it('reads only the results the callback sends', () => {
		expect(resultOf('connected')).toBe('connected');
		expect(resultOf('refused')).toBe('refused');
		expect(resultOf('failed')).toBe('failed');
		expect(resultOf('<script>')).toBeNull();
		expect(resultOf(null)).toBeNull();
		expect(resultLine('connected', 'simonhull')).toBe('Connected as simonhull');
		expect(resultLine('refused', undefined)).toBe('Not connected: you said no on last.fm.');
	});
});
