import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createFixtureSource } from '#lib/library/fixtures.ts';
import { Library } from '#lib/library/library.svelte.ts';
import type { LibrarySource, PlaybackRecord, PlaybackSave } from '#lib/library/source.ts';
import { Shuffles } from '#lib/shuffle/session.svelte.ts';
import { SilentOutput } from './output.ts';
import { Player } from './player.svelte.ts';
import {
	pausedSaveMs,
	Resume,
	seekedSaveMs,
	soundedSaveMs,
	soundingSaveMs
} from './resume.svelte.ts';

const album = 'glasshouse-66';
const here = { id: '0123456789abcdef', device: 'computer' } as const;

function record(extra: Partial<PlaybackRecord> = {}): PlaybackRecord {
	return {
		kind: 'album',
		tracks: Array.from({ length: 10 }, (_, at) => ({
			id: `${album}-${String(at + 1)}`,
			albumId: album
		})),
		index: 3,
		positionMs: 133_000,
		ordered: true,
		moved: false,
		clientId: 'feedfacecafebeef',
		device: 'phone',
		updatedAt: 1000,
		...extra
	};
}

async function setup(answer: () => Promise<PlaybackRecord | null> = () => Promise.resolve(null)) {
	const saves: { save: PlaybackSave; keepalive: boolean }[] = [];
	const playback = vi.fn(answer);
	const save = vi.fn((sent: PlaybackSave, { keepalive }: { keepalive: boolean }) => {
		saves.push({ save: sent, keepalive });
		return Promise.resolve();
	});
	const source: LibrarySource = {
		...createFixtureSource(),
		streamUrl: (id: string) => id,
		playback,
		savePlayback: save
	};
	const library = new Library(source, { simulated: true, events: null });
	vi.spyOn(library, 'reportPlay').mockImplementation(() => undefined);
	vi.spyOn(library, 'reportListen').mockImplementation(() => undefined);
	library.user = await source.me();
	const output = new SilentOutput({ ticking: false });
	const player = new Player(library, () => output);
	const shuffles = new Shuffles(library, player);
	const resume = new Resume({ player, library, shuffles, client: () => here });
	return { library, player, output, resume, saves, save, playback };
}

/** Lets everything waiting on a resolved promise run, without moving the clock. */
async function settle(): Promise<void> {
	await vi.advanceTimersByTimeAsync(0);
}

beforeEach(() => {
	vi.useFakeTimers();
});

afterEach(() => {
	vi.useRealTimers();
});

describe('restoring', () => {
	it('comes back paused, where it was left, and saves nothing for it', async () => {
		const { player, library, resume, save } = await setup(() => Promise.resolve(record()));
		await resume.restore();
		expect(player.status).toBe('paused');
		expect(player.current?.id).toBe(`${album}-4`);
		expect(player.position).toBe(133);
		expect(library.playing?.track.id).toBe(`${album}-4`);
		expect(resume.arrival).toBe('restore');
		expect(resume.left).toMatchObject({ form: 'phone', moved: false });
		await vi.advanceTimersByTimeAsync(60_000);
		await resume.flush({ keepalive: true });
		expect(save).not.toHaveBeenCalled();
		expect(resume.unsaved).toBe(false);
	});

	it('says nothing of a record this browser saved', async () => {
		const { resume } = await setup(() => Promise.resolve(record({ clientId: here.id })));
		await resume.restore();
		expect(resume.left).toBeNull();
	});

	it('asks once per sign-in', async () => {
		const { resume, playback } = await setup();
		await resume.restore();
		await resume.restore();
		expect(playback).toHaveBeenCalledOnce();
		resume.reset();
		await resume.restore();
		expect(playback).toHaveBeenCalledTimes(2);
	});

	it('drops a record that arrives after something has started', async () => {
		let answer: (value: PlaybackRecord) => void = () => undefined;
		const { player, library, resume } = await setup(() => new Promise((done) => (answer = done)));
		const restoring = resume.restore();
		await library.loadAlbum('night-ferry-94');
		const other = library.album('night-ferry-94');
		if (other === undefined) {
			throw new Error('no album');
		}
		player.play(other);
		answer(record());
		await restoring;
		expect(player.queue?.album.id).toBe('night-ferry-94');
		expect(resume.arrival).toBeNull();
	});
});

describe('saving', () => {
	async function playing() {
		const context = await setup();
		await context.resume.restore();
		await context.library.loadAlbum(album);
		const held = context.library.album(album);
		if (held === undefined) {
			throw new Error('no album');
		}
		context.player.play(held, 2);
		await settle();
		return context;
	}

	it(`saves ${String(soundedSaveMs)} ms after a song first sounds`, async () => {
		const { saves, player } = await playing();
		expect(player.status).toBe('playing');
		await vi.advanceTimersByTimeAsync(soundedSaveMs - 1);
		expect(saves).toEqual([]);
		await vi.advanceTimersByTimeAsync(1);
		expect(saves).toHaveLength(1);
		expect(saves[0]?.save).toMatchObject({
			kind: 'album',
			index: 2,
			clientId: here.id,
			device: 'computer'
		});
		expect(saves[0]?.keepalive).toBe(false);
	});

	it(`saves ${String(pausedSaveMs)} ms after a pause`, async () => {
		const { saves, player, output } = await playing();
		output.elapse(40);
		player.pause();
		await vi.advanceTimersByTimeAsync(pausedSaveMs);
		expect(saves).toHaveLength(1);
		expect(saves[0]?.save.positionMs).toBe(40_000);
	});

	it('waits for seeks to settle', async () => {
		const { saves, player } = await playing();
		player.pause();
		await vi.advanceTimersByTimeAsync(pausedSaveMs);
		player.seek(10);
		await vi.advanceTimersByTimeAsync(seekedSaveMs / 2);
		player.seek(20);
		await vi.advanceTimersByTimeAsync(seekedSaveMs - 1);
		expect(saves).toHaveLength(1);
		await vi.advanceTimersByTimeAsync(1);
		expect(saves).toHaveLength(2);
		expect(saves[1]?.save.positionMs).toBe(20_000);
	});

	it(`saves every ${String(soundingSaveMs / 1000)} s while sounding, and not while paused`, async () => {
		const { saves, player } = await playing();
		await vi.advanceTimersByTimeAsync(soundedSaveMs);
		expect(saves).toHaveLength(1);
		await vi.advanceTimersByTimeAsync(soundingSaveMs);
		expect(saves.length).toBeGreaterThanOrEqual(2);
		player.pause();
		await vi.advanceTimersByTimeAsync(pausedSaveMs);
		const paused = saves.length;
		await vi.advanceTimersByTimeAsync(soundingSaveMs * 3);
		expect(saves).toHaveLength(paused);
	});

	it('flushes with keepalive when the page goes, only with something to save', async () => {
		const { saves, resume, player } = await playing();
		await resume.flush({ keepalive: true });
		expect(saves.at(-1)?.keepalive).toBe(true);
		player.pause();
		await resume.flush({ keepalive: true });
		const sent = saves.length;
		await resume.flush({ keepalive: true });
		expect(saves).toHaveLength(sent);
	});

	it('keeps a failed save to go again', async () => {
		const { resume, save, player } = await playing();
		save.mockImplementationOnce(() => Promise.reject(new Error('down')));
		player.pause();
		await resume.flush();
		expect(resume.dirty).toBe(true);
		await resume.flush();
		expect(save).toHaveBeenCalledTimes(2);
		expect(resume.dirty).toBe(false);
	});

	it('saves nothing new for a player that failed', async () => {
		const { resume, save, player } = await playing();
		player.failure = true;
		player.pause();
		await vi.advanceTimersByTimeAsync(soundedSaveMs);
		await resume.flush();
		expect(save).toHaveBeenCalledTimes(0);
	});

	it('is unsaved from a change until the server holds it', async () => {
		const { resume, player } = await playing();
		expect(resume.unsaved).toBe(true);
		player.pause();
		await vi.advanceTimersByTimeAsync(pausedSaveMs);
		expect(resume.unsaved).toBe(false);
	});
});

describe('catching up', () => {
	it('takes a newer record into a tab left paused and untouched', async () => {
		let next = record();
		const { resume, player } = await setup(() => Promise.resolve(next));
		await resume.restore();
		next = record({ index: 6, positionMs: 5000, updatedAt: 2000 });
		await resume.catchUp();
		expect(player.current?.id).toBe(`${album}-7`);
		expect(player.position).toBe(5);
		expect(player.status).toBe('paused');
	});

	it('leaves an older record, and a tab that has played', async () => {
		let next = record();
		const { resume, player } = await setup(() => Promise.resolve(next));
		await resume.restore();
		await resume.catchUp();
		expect(player.current?.id).toBe(`${album}-4`);
		player.toggle();
		await settle();
		player.pause();
		next = record({ index: 6, updatedAt: 2000 });
		await resume.catchUp();
		expect(player.current?.id).toBe(`${album}-4`);
	});
});

describe('reset', () => {
	it('forgets the record, and what was left unsaved', async () => {
		const { resume } = await setup(() => Promise.resolve(record()));
		await resume.restore();
		resume.reset();
		expect(resume.left).toBeNull();
		expect(resume.arrival).toBeNull();
		expect(resume.dirty).toBe(false);
	});
});
