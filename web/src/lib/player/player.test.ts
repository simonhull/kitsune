import { describe, expect, it, vi } from 'vitest';
import { createFixtureSource } from '#lib/library/fixtures.ts';
import { Library } from '#lib/library/library.svelte.ts';
import {
	ServerError,
	UnauthorizedError,
	type Album,
	type LibrarySource,
	type Track
} from '#lib/library/source.ts';
import { SilentOutput } from './output.ts';
import { Player } from './player.svelte.ts';

function track(index: number, extra: Partial<Track> = {}): Track {
	return {
		id: `t${String(index)}`,
		title: `T${String(index)}`,
		artists: [],
		durationMs: 100_000,
		format: 'FLAC',
		...extra
	};
}

function album(count: number, id = 'a', extra: Partial<Track> = {}): Album {
	return {
		id,
		title: id,
		artist: 'B',
		trackCount: count,
		compilation: false,
		tracks: Array.from({ length: count }, (_, index) => track(index, extra)),
		durationMs: 1000,
		formats: ['FLAC'],
		addedAt: '2026-10-12T18:00:00Z'
	};
}

/** Answers each `play` as told, then plays for real. */
class ScriptedOutput extends SilentOutput {
	readonly answers: (() => Promise<void>)[] = [];
	mediaTypes = new Set(['audio/flac']);

	constructor() {
		super({ ticking: false });
	}

	override canPlay(mediaType?: string): boolean {
		return mediaType === undefined || this.mediaTypes.has(mediaType);
	}

	override play(): Promise<void> {
		return this.answers.shift()?.() ?? super.play();
	}

	fail(): void {
		this.events?.failed();
	}

	stopFromOutside(): void {
		this.pause();
		this.events?.stopped();
	}
}

function refusal(name: string): () => Promise<void> {
	return () => Promise.reject(new DOMException('refused', name));
}

function setup(overrides: Partial<LibrarySource> = {}) {
	const source = { ...createFixtureSource(), streamUrl: (id: string) => id, ...overrides };
	const library = new Library(source);
	const output = new ScriptedOutput();
	const create = vi.fn(() => output);
	return { library, output, create, player: new Player(library, create) };
}

/** Lets everything waiting on a resolved promise run. */
async function settle(): Promise<void> {
	await new Promise((done) => setTimeout(done));
}

describe('Player', () => {
	it('builds nothing until the first play', async () => {
		const { player, create } = setup();
		expect(create).not.toHaveBeenCalled();
		player.play(album(2));
		await settle();
		expect(create).toHaveBeenCalledOnce();
	});

	it('plays an album from the track asked for', async () => {
		const { player, output } = setup();
		player.play(album(3), 1);
		await settle();
		expect(player.status).toBe('playing');
		expect(player.current?.id).toBe('t1');
		expect(output.url).toBe('t1');
		expect(player.started).toBe(true);
	});

	it('moves position with the clock', async () => {
		const { player, output } = setup();
		player.play(album(1));
		await settle();
		output.elapse(12);
		expect(player.position).toBe(12);
	});

	it('moves to the next track the moment one ends', async () => {
		const { player, output } = setup();
		player.play(album(2));
		await settle();
		output.elapse(100);
		expect(player.current?.id).toBe('t1');
		expect(output.url).toBe('t1');
		expect(player.position).toBe(0);
	});

	it('stops paused at the first track at the end of the queue, and stays started', async () => {
		const { player, output } = setup();
		player.play(album(2), 1);
		await settle();
		output.elapse(100);
		expect(player.status).toBe('paused');
		expect(player.current?.id).toBe('t0');
		expect(player.position).toBe(0);
		expect(player.started).toBe(true);
		expect(output.playing).toBe(false);

		player.resume();
		await settle();
		expect(output.url).toBe('t0');
		expect(player.status).toBe('playing');
	});

	it('sends the next track to preload with 15 seconds left, once', async () => {
		const { player, output } = setup();
		const preload = vi.spyOn(output, 'preload');
		player.play(album(2));
		await settle();
		output.elapse(84);
		expect(preload).not.toHaveBeenCalled();
		output.elapse(1);
		expect(output.preloaded).toBe('t1');
		output.elapse(1);
		expect(preload).toHaveBeenCalledOnce();
	});

	it('preloads straight away under a short track', async () => {
		const { player, output } = setup();
		player.play(album(2, 'a', { durationMs: 10_000 }));
		await settle();
		output.elapse(1);
		expect(output.preloaded).toBe('t1');
	});

	it('toggles', async () => {
		const { player, output } = setup();
		player.play(album(1));
		await settle();
		player.toggle();
		expect(player.status).toBe('paused');
		expect(output.playing).toBe(false);
		player.toggle();
		await settle();
		expect(player.status).toBe('playing');
	});

	it('toggles the album in the player and starts any other', async () => {
		const { player } = setup();
		const first = album(2, 'first');
		player.playOrToggle(first);
		await settle();
		player.playOrToggle(first);
		expect(player.status).toBe('paused');
		player.playOrToggle(album(2, 'second'));
		await settle();
		expect(player.queue?.album.id).toBe('second');
		expect(player.status).toBe('playing');
	});

	it('restarts past 3 seconds, and steps back before that', async () => {
		const { player, output } = setup();
		player.play(album(3), 1);
		await settle();
		output.elapse(4);
		player.previous();
		expect(player.current?.id).toBe('t1');
		expect(output.position).toBe(0);
		output.elapse(2);
		player.previous();
		expect(player.current?.id).toBe('t0');
	});

	it('restarts the first track instead of stepping off the queue', async () => {
		const { player, output } = setup();
		player.play(album(2));
		await settle();
		output.elapse(1);
		player.previous();
		expect(player.current?.id).toBe('t0');
		expect(output.position).toBe(0);
	});

	it('goes past the last track to the end of the queue', async () => {
		const { player } = setup();
		player.play(album(2), 1);
		await settle();
		player.next();
		expect(player.current?.id).toBe('t0');
		expect(player.status).toBe('paused');
	});

	it('seeks within the track', async () => {
		const { player, output } = setup();
		player.play(album(1));
		await settle();
		player.seek(40);
		expect(output.position).toBe(40);
		expect(player.position).toBe(40);
		player.seek(9999);
		expect(player.position).toBe(100);
		player.seek(-3);
		expect(player.position).toBe(0);
	});

	it('shuffles every track of the album', async () => {
		const { player } = setup();
		player.shuffle(album(12));
		await settle();
		expect(player.queue?.ordered).toBe(false);
		expect(new Set(player.queue?.tracks).size).toBe(12);
		expect(player.queue?.tracks.at(0)).toBe(player.current);
	});

	it('plays at album gain in order and track gain shuffled', async () => {
		const { player, output } = setup();
		const gained = album(2, 'a', { replayGain: { albumGain: -3, trackGain: -8 } });
		player.play(gained);
		await settle();
		expect(output.gain).toBe(-3);
		player.shuffle(gained);
		await settle();
		expect(output.gain).toBe(-8);
	});

	it('plays an untagged track as it is', async () => {
		const { player, output } = setup();
		player.play(album(2, 'a', { replayGain: { trackGain: -8 } }));
		await settle();
		player.play(album(1, 'b'));
		await settle();
		expect(output.gain).toBe(0);
	});
});

describe('Player failures', () => {
	it('ignores a superseded play', async () => {
		const { player, output } = setup();
		let reject: (error: Error) => void = () => undefined;
		output.answers.push(
			() =>
				new Promise((_, refuse) => {
					reject = refuse;
				})
		);
		player.play(album(1, 'first'));
		player.play(album(1, 'second'));
		await settle();
		reject(new DOMException('superseded', 'AbortError'));
		await settle();
		expect(player.queue?.album.id).toBe('second');
		expect(player.status).toBe('playing');
		expect(player.failure).toBe(false);
	});

	it('ignores a late refusal of a play already replaced', async () => {
		const { player, output } = setup();
		let reject: (error: Error) => void = () => undefined;
		output.answers.push(
			() =>
				new Promise((_, refuse) => {
					reject = refuse;
				})
		);
		player.play(album(2));
		player.next();
		await settle();
		reject(new DOMException('gone', 'NotSupportedError'));
		await settle();
		expect(player.current?.id).toBe('t1');
		expect(player.status).toBe('playing');
	});

	it('sits paused without a word when the browser wants a gesture', async () => {
		const { player, output } = setup();
		output.answers.push(refusal('NotAllowedError'));
		player.play(album(2));
		await settle();
		expect(player.status).toBe('paused');
		expect(player.failure).toBe(false);
		expect(player.current?.id).toBe('t0');
	});

	it('skips a track the server says is gone', async () => {
		const { player, output } = setup();
		output.answers.push(refusal('NotSupportedError'));
		player.play(album(3));
		await settle();
		expect(player.current?.id).toBe('t1');
		expect(player.status).toBe('playing');
	});

	it('counts an error event and a rejection as one failure', async () => {
		const { player, output } = setup();
		output.answers.push(refusal('NotSupportedError'));
		player.play(album(3));
		output.fail();
		await settle();
		expect(player.current?.id).toBe('t1');
	});

	it('stops after three failures in a row, and stays started', async () => {
		const { player, output } = setup();
		output.answers.push(...Array.from({ length: 3 }, () => refusal('NotSupportedError')));
		player.play(album(6));
		await settle();
		expect(player.failure).toBe(true);
		expect(player.status).toBe('paused');
		expect(player.current?.id).toBe('t2');
		expect(player.started).toBe(true);
	});

	it('starts counting again after a track plays', async () => {
		const { player, output } = setup();
		const fails = refusal('NotSupportedError');
		output.answers.push(fails, fails, () => SilentOutput.prototype.play.call(output), fails, fails);
		player.play(album(8));
		await settle();
		output.elapse(100);
		await settle();
		expect(player.failure).toBe(false);
		expect(player.current?.id).toBe('t5');
	});

	it('stops with a word when the last track fails: there is nothing to skip to', async () => {
		const { player, output } = setup();
		output.answers.push(refusal('NotSupportedError'));
		player.play(album(1));
		await settle();
		expect(player.failure).toBe(true);
		expect(player.status).toBe('paused');
	});

	it('clears the word when someone tries again', async () => {
		const { player, output } = setup();
		output.answers.push(refusal('NotSupportedError'));
		player.play(album(1));
		await settle();
		player.toggle();
		await settle();
		expect(player.failure).toBe(false);
		expect(player.status).toBe('playing');
		expect(output.url).toBe('t0');
	});

	it.each([
		['the server is down', new TypeError('offline')],
		['the server answers badly', new ServerError(502)]
	])('pauses and keeps its place when %s', async (_, reason) => {
		let answer: Promise<never> | null = null;
		const { player, output } = setup({
			me: () => answer ?? createFixtureSource().me()
		});
		player.play(album(4), 1);
		await settle();
		output.elapse(30);
		answer = Promise.reject(reason);
		answer.catch(() => undefined);
		output.fail();
		await settle();
		expect(player.status).toBe('paused');
		expect(player.current?.id).toBe('t1');
		expect(player.position).toBe(30);
		expect(player.failure).toBe(false);

		answer = null;
		const load = vi.spyOn(output, 'load');
		player.toggle();
		await settle();
		expect(load).toHaveBeenCalledOnce();
		expect(output.position).toBe(30);
		expect(player.status).toBe('playing');
	});

	it('burns nothing while a restart fails every track', async () => {
		const { player, output } = setup({ me: () => Promise.reject(new TypeError('offline')) });
		output.answers.push(refusal('NotSupportedError'));
		player.play(album(4));
		await settle();
		expect(player.current?.id).toBe('t0');
		expect(player.status).toBe('paused');
	});

	it('pauses on a lost session and leaves the door to the library', async () => {
		const { player, output } = setup({ me: () => Promise.reject(new UnauthorizedError()) });
		output.answers.push(refusal('NotSupportedError'));
		player.play(album(4));
		await settle();
		expect(player.status).toBe('paused');
		expect(player.current?.id).toBe('t0');
	});

	it('leaves a paused player paused when its media dies', async () => {
		const { player, output } = setup();
		player.play(album(3));
		await settle();
		player.pause();
		output.fail();
		await settle();
		expect(player.status).toBe('paused');
		expect(player.current?.id).toBe('t0');
	});

	it('skips what the browser says it cannot play, without asking the server', async () => {
		const me = vi.fn(createFixtureSource().me);
		const { player, output } = setup({ me });
		const mixed = album(3);
		const tracks = [track(0, { format: 'AIFF' }), track(1, { format: 'AIFF' }), track(2)];
		player.play({ ...mixed, tracks });
		await settle();
		expect(player.current?.id).toBe('t2');
		expect(output.url).toBe('t2');
		expect(me).not.toHaveBeenCalled();
	});

	it('tries a format nobody can vouch for', async () => {
		const { player, output } = setup();
		player.play(album(1, 'a', { format: 'Unknown' }));
		await settle();
		expect(output.url).toBe('t0');
	});

	it('pauses when the output stops by itself, and doesn’t restart when told again', async () => {
		const { player, output } = setup();
		player.play(album(2));
		await settle();
		output.stopFromOutside();
		expect(player.status).toBe('paused');
		output.stopFromOutside();
		expect(player.status).toBe('paused');
	});
});

describe('Player reset', () => {
	it('clears the queue and silences the output, without building another', async () => {
		const { library, player, output, create } = setup();
		player.play(album(3), 1);
		await settle();
		output.elapse(5);
		library.reset();
		expect(player.queue).toBeNull();
		expect(player.current).toBeNull();
		expect(player.status).toBe('idle');
		expect(player.position).toBe(0);
		expect(player.started).toBe(false);
		expect(output.playing).toBe(false);
		expect(output.url).toBe('');

		player.play(album(1));
		await settle();
		expect(create).toHaveBeenCalledOnce();
	});

	it('leaves nothing for a late answer to bring back', async () => {
		let answer: (session: never) => void = () => undefined;
		const { library, player, output } = setup({
			me: () =>
				new Promise((resolve) => {
					answer = resolve;
				})
		});
		output.answers.push(refusal('NotSupportedError'));
		player.play(album(3));
		await settle();
		library.reset();
		answer(undefined as never);
		await settle();
		expect(player.queue).toBeNull();
		expect(output.url).toBe('');
	});
});
