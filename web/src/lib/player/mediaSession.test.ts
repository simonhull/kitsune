import { describe, expect, it } from 'vitest';
import { createFixtureSource } from '#lib/library/fixtures.ts';
import { Library } from '#lib/library/library.svelte.ts';
import type { Album } from '#lib/library/source.ts';
import { followMediaSession, metadataOf, playbackOf, positionOf } from './mediaSession.svelte';
import { SilentOutput } from './output';
import { Player } from './player.svelte';

const album: Album = {
	id: 'a',
	title: 'A',
	artist: 'B',
	artists: [],
	trackCount: 2,
	compilation: false,
	artwork: { hash: 'abc', accent: '#112233', ground: '#000000', width: 1, height: 1 },
	tracks: [
		{
			id: 't0',
			title: 'T0',
			artists: ['Guest'],
			durationMs: 100_000,
			format: 'FLAC',
			genres: [],
			etag: '',
			hearted: false
		},
		{
			id: 't1',
			title: 'T1',
			artists: [],
			durationMs: 200_000,
			format: 'FLAC',
			genres: [],
			etag: '',
			hearted: false
		}
	],
	discs: [],
	durationMs: 300_000,
	formats: ['FLAC'],
	addedAt: '2026-10-12T18:00:00Z',
	genres: [],
	etag: '',
	hearted: false
};

class FakeSession {
	handlers = new Map<string, MediaSessionActionHandler | null>();

	setActionHandler(action: string, handler: MediaSessionActionHandler | null): void {
		this.handlers.set(action, handler);
	}

	act(action: string, details: Partial<MediaSessionActionDetails> = {}): void {
		this.handlers.get(action)?.({ action, ...details } as MediaSessionActionDetails);
	}
}

function setup() {
	const output = new SilentOutput({ ticking: false });
	return { output, player: new Player(new Library(createFixtureSource()), () => output) };
}

describe('what the system is told', () => {
	it('names the track sounding', () => {
		const { player } = setup();
		player.play(album);
		expect(metadataOf(player)).toMatchObject({
			title: 'T0',
			artist: 'Guest',
			album: 'A',
			artwork: [{ src: '/api/v1/artwork/abc?size=512' }]
		});
		player.next();
		expect(metadataOf(player)).toMatchObject({ title: 'T1', artist: 'B' });
	});

	it('says what state it is in', () => {
		const { player } = setup();
		expect(playbackOf(player)).toBe('none');
		player.play(album);
		expect(playbackOf(player)).toBe('playing');
		player.pause();
		expect(playbackOf(player)).toBe('paused');
	});

	it('keeps the position honest', () => {
		const { output, player } = setup();
		player.play(album);
		output.elapse(12);
		expect(positionOf(player)).toEqual({ duration: 100, position: 12, playbackRate: 1 });
	});

	it('forgets everything after a reset', () => {
		const { player } = setup();
		player.play(album);
		player.reset();
		expect(metadataOf(player)).toBeNull();
		expect(playbackOf(player)).toBe('none');
		expect(positionOf(player)).toBeUndefined();
	});
});

describe('followMediaSession', () => {
	it('takes the actions, and lets go when it leaves', () => {
		const { player } = setup();
		const session = new FakeSession();
		const leave = followMediaSession(player, session as unknown as MediaSession);
		player.play(album);
		session.act('pause');
		expect(player.status).toBe('paused');
		session.act('play');
		expect(player.sounding).toBe(true);
		session.act('nexttrack');
		expect(player.current?.id).toBe('t1');
		session.act('previoustrack');
		expect(player.current?.id).toBe('t0');
		session.act('seekto', { seekTime: 30 });
		expect(player.position).toBe(30);
		leave();
		expect(session.handlers.get('play')).toBeNull();
	});

	it('stays out of a browser with no media session', () => {
		const { player } = setup();
		const leave = followMediaSession(player, undefined);
		expect(leave()).toBeUndefined();
	});
});
