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
		genres: [],
		etag: '',
		hearted: false,
		...extra
	};
}

function album(count: number, id = 'a', extra: Partial<Track> = {}): Album {
	return {
		id,
		title: id,
		artist: 'B',
		artists: [],
		trackCount: count,
		compilation: false,
		tracks: Array.from({ length: count }, (_, index) => track(index, extra)),
		discs: [],
		durationMs: 1000,
		formats: ['FLAC'],
		addedAt: '2026-10-12T18:00:00Z',
		genres: [],
		etag: '',
		hearted: false
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
	const reported = vi.spyOn(library, 'reportPlay').mockImplementation(() => undefined);
	return { library, output, create, reported, player: new Player(library, create) };
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

	it('plays one disc of an album, and nothing for a disc it lacks', async () => {
		const { player, output } = setup();
		const two = album(4);
		const discs = {
			...two,
			tracks: two.tracks.map((track, index) => ({ ...track, discNo: index < 2 ? 1 : 2 }))
		};
		player.playDisc(discs, 3);
		await settle();
		expect(player.queue).toBeNull();
		player.playDisc(discs, 2);
		await settle();
		expect(player.current?.id).toBe('t2');
		expect(output.url).toBe('t2');
		expect(player.queue?.tracks.map(({ id }) => id)).toEqual(['t2', 't3']);
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
		expect(player.finished).toBe(true);
		expect(output.playing).toBe(false);

		player.resume();
		await settle();
		expect(output.url).toBe('t0');
		expect(player.status).toBe('playing');
		expect(player.finished).toBe(false);
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

	it('preloads again after going back over the same ground', async () => {
		const { player, output } = setup();
		const preload = vi.spyOn(output, 'preload');
		player.play(album(3), 1);
		await settle();
		output.elapse(90);
		player.previous();
		player.seek(0);
		player.play(album(3), 1);
		await settle();
		output.elapse(90);
		expect(preload).toHaveBeenCalledTimes(2);
	});

	it('preloads straight away under a short track', async () => {
		const { player, output } = setup();
		player.play(album(2, 'a', { durationMs: 10_000 }));
		await settle();
		output.elapse(1);
		expect(output.preloaded).toBe('t1');
	});

	it('says each listen is playing once, when its sound starts', async () => {
		const { player, output, library } = setup();
		const announced = vi.spyOn(library, 'nowPlaying').mockImplementation(() => undefined);
		player.play(album(2));
		await settle();
		expect(announced).toHaveBeenCalledExactlyOnceWith('t0');
		player.toggle();
		player.toggle();
		await settle();
		expect(announced).toHaveBeenCalledOnce();
		output.elapse(100);
		await settle();
		expect(announced).toHaveBeenLastCalledWith('t1');
		expect(announced).toHaveBeenCalledTimes(2);
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

	it('plays an album from a list on through it, and toggles it while it sounds there', async () => {
		const { player } = setup();
		const a = album(2, 'a');
		player.playOrToggle(a, 'list');
		await settle();
		expect(player.queue?.continues).toEqual({ album: 'a' });
		expect(player.current?.id).toBe('t0');
		player.playOrToggle(a, 'list');
		expect(player.status).toBe('paused');
		player.playOrToggle(a, 'list');
		await settle();
		expect(player.status).toBe('playing');
		// Another album from elsewhere comes to rest after it.
		player.playOrToggle(album(2, 'b'));
		await settle();
		expect(player.queue?.continues).toBeUndefined();
	});

	it('plays picked songs, puts songs next and adds them at the end', async () => {
		const { player } = setup();
		const a = album(2, 'a');
		const b = {
			...album(2, 'b'),
			tracks: album(2, 'b').tracks.map((t) => ({ ...t, id: `b${t.id}` }))
		};
		const song = (from: Album, index: number) => ({
			album: from,
			track: from.tracks[index] as Track
		});
		player.addToQueue([song(b, 1)]);
		await settle();
		expect(player.current?.id).toBe('bt1');
		expect(player.album?.id).toBe('b');
		player.playNext([song(a, 0)]);
		player.addToQueue([song(a, 1)]);
		expect(player.queue?.tracks.map(({ id }) => id)).toEqual(['bt1', 't0', 't1']);
		expect(player.current?.id).toBe('bt1');
		player.next();
		await settle();
		expect(player.album?.id).toBe('a');
		expect(player.cue?.album.id).toBe('a');
		player.playSongs([song(b, 0), song(a, 1)]);
		await settle();
		expect(player.current?.id).toBe('bt0');
		expect(player.queue?.ordered).toBe(false);
	});

	it('plays an album over a queue of picked songs that starts with it', async () => {
		const { player } = setup();
		const a = album(2, 'a');
		player.playSongs([{ album: a, track: a.tracks[1] as Track }]);
		await settle();
		player.playOrToggle(a);
		await settle();
		expect(player.queue?.albums).toBeUndefined();
		expect(player.current?.id).toBe('t0');
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
		const gained = album(2, 'a', { replayGain: { albumGain: -3, trackGain: -8, source: 'tags' } });
		player.play(gained);
		await settle();
		expect(output.gain).toBe(-3);
		player.shuffle(gained);
		await settle();
		expect(output.gain).toBe(-8);
	});

	it('plays an untagged track as it is', async () => {
		const { player, output } = setup();
		player.play(album(2, 'a', { replayGain: { trackGain: -8, source: 'tags' } }));
		await settle();
		player.play(album(1, 'b'));
		await settle();
		expect(output.gain).toBe(0);
	});

	it('plays a measured gain as a tagged one, and turns down a song with none', async () => {
		const { library, output, create } = setup();
		let untagged = -8;
		const player = new Player(library, create, () => untagged);
		player.play(
			album(1, 'a', { replayGain: { trackGain: -12, trackPeak: 1.1, source: 'measured' } })
		);
		await settle();
		expect(output.gain).toBe(-12);
		player.play(album(1, 'b'));
		await settle();
		expect(output.gain).toBe(-8);
		untagged = -3; // Read as each song loads.
		player.play(album(1, 'c'));
		await settle();
		expect(output.gain).toBe(-3);
	});
});

describe('the transport before the first play', () => {
	async function cued() {
		const context = setup();
		context.library.cueAlbum(album(4));
		await settle();
		return context;
	}

	it('plays what is cued', async () => {
		const { player } = await cued();
		player.toggle();
		await settle();
		expect(player.current?.id).toBe('t0');
		expect(player.status).toBe('playing');
	});

	it('starts one track in on Next, and at the top on Previous', async () => {
		const forward = await cued();
		forward.player.next();
		expect(forward.player.current?.id).toBe('t1');
		const back = await cued();
		back.player.previous();
		expect(back.player.current?.id).toBe('t0');
	});

	it('does nothing with nothing cued', () => {
		const { player } = setup();
		player.toggle();
		player.next();
		player.previous();
		player.seek(10);
		expect(player.queue).toBeNull();
		expect(player.status).toBe('idle');
	});

	it('leaves a single on its one track', async () => {
		const { library, player } = setup();
		library.cueAlbum(album(1));
		player.next();
		await settle();
		expect(player.current?.id).toBe('t0');
	});
});

describe('a track change', () => {
	it('moves the sound at once and leaves the names to the shell', async () => {
		const { library, player, output } = setup();
		const waiting: (() => void)[] = [];
		player.play(album(3));
		await settle();
		player.carry = (apply) => waiting.push(apply);
		player.next();
		expect(output.url).toBe('t1');
		expect(player.current?.id).toBe('t1');
		expect(player.position).toBe(0);
		expect(library.playing?.track.id).toBe('t0');
		waiting.at(0)?.();
		expect(library.playing?.track.id).toBe('t1');
	});

	it('shows the latest track, however late it gets to', async () => {
		const { library, player } = setup();
		const waiting: (() => void)[] = [];
		player.play(album(4));
		await settle();
		player.carry = (apply) => waiting.push(apply);
		player.next();
		player.next();
		waiting.at(0)?.();
		expect(library.playing?.track.id).toBe('t2');
	});

	it('shows nothing after a reset, however late', async () => {
		const { library, player } = setup();
		const waiting: (() => void)[] = [];
		player.play(album(3));
		await settle();
		player.carry = (apply) => waiting.push(apply);
		player.next();
		library.reset();
		waiting.at(0)?.();
		expect(library.playing).toBeNull();
	});

	it('has nothing to carry when a queue starts on the cued track', async () => {
		const { library, player } = setup();
		const carry = vi.fn();
		library.cueAlbum(album(3));
		player.carry = carry;
		player.play(album(3));
		await settle();
		expect(carry).not.toHaveBeenCalled();
		expect(library.playing?.index).toBe(0);
	});
});

describe('the cue under a player', () => {
	it('names the queue and what follows it when shuffled', async () => {
		const { library, player } = setup();
		player.shuffle(album(6));
		await settle();
		const order = player.queue?.tracks ?? [];
		expect(library.playing?.track).toBe(order.at(0));
		expect(library.playing?.upNext).toEqual(order.slice(1, 4));
		expect(library.playing?.moreCount).toBe(2);
		expect(library.playing?.trackCount).toBe(6);
		player.next();
		await settle();
		expect(library.playing?.index).toBe(1);
		expect(library.playing?.upNext).toEqual(order.slice(2, 5));
	});

	it('still follows the album opened until the first play', () => {
		const { library } = setup();
		library.cueAlbum(album(2, 'first'));
		library.cueAlbum(album(2, 'second'));
		expect(library.playing?.album.id).toBe('second');
	});

	it('leaves cue and sound alone when another album opens during playback', async () => {
		const { library, player, output } = setup();
		player.play(album(4), 2);
		await settle();
		library.cueAlbum(album(3, 'other'));
		expect(library.playing?.album.id).toBe('a');
		expect(library.playing?.track.id).toBe('t2');
		expect(player.queue?.album.id).toBe('a');
		expect(output.url).toBe('t2');
		expect(output.playing).toBe(true);
	});

	it.each([
		['paused', (player: Player) => player.pause()],
		['past the end of the queue', (player: Player) => player.next()]
	])('stays started when %s', async (_, happen) => {
		const { library, player } = setup();
		player.play(album(2), 1);
		await settle();
		happen(player);
		library.cueAlbum(album(3, 'other'));
		expect(library.playing?.album.id).toBe('a');
	});

	it('takes over from the pin', async () => {
		const pinned = new Library(createFixtureSource(), { pinnedAlbumId: 'night-ferry-94' });
		await pinned.loadShell();
		expect(pinned.playing?.album.id).toBe('night-ferry-94');
		const under = new Player(pinned, () => new ScriptedOutput());
		under.play(album(3, 'glass'), 1);
		await settle();
		expect(pinned.playing?.album.id).toBe('glass');
		expect(pinned.playing?.index).toBe(1);
	});

	it('leaves nothing in the island after a reset', async () => {
		const { library, player } = setup();
		player.play(album(2));
		await settle();
		library.reset();
		expect(library.playing).toBeNull();
		library.cueAlbum(album(2, 'next-user'));
		expect(library.playing?.album.id).toBe('next-user');
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

	it('picks up where it was when a file changes under a track', async () => {
		const { player, output } = setup();
		player.play(album(2));
		await settle();
		output.elapse(40);
		output.fail();
		await settle();
		expect(player.current?.id).toBe('t0');
		expect(output.url).toBe('t0');
		expect(output.position).toBe(40);
		expect(player.position).toBe(40);
		expect(player.status).toBe('playing');
		expect(player.failure).toBe(false);
	});

	it('shows no trip through the start on the way back', async () => {
		const { player, output } = setup();
		player.play(album(2));
		await settle();
		output.elapse(40);
		output.fail();
		await settle();
		output.seek(0);
		expect(player.position).toBe(40);
		output.seek(41);
		expect(player.position).toBe(41);
	});

	it('moves on when the reload fails too: the file has gone', async () => {
		const { player, output } = setup();
		player.play(album(2));
		await settle();
		output.elapse(40);
		output.fail();
		await settle();
		output.fail();
		await settle();
		expect(player.current?.id).toBe('t1');
	});

	it('spends the reload until the track has played on from it', async () => {
		const { player, output } = setup();
		player.play(album(2));
		await settle();
		output.elapse(40);
		output.fail();
		await settle();
		output.elapse(10);
		output.fail();
		await settle();
		expect(player.current?.id).toBe('t0');
		expect(output.position).toBe(50);
	});

	it('goes where someone seeks, straight after a reload', async () => {
		const { player, output } = setup();
		player.play(album(2));
		await settle();
		output.elapse(40);
		output.fail();
		await settle();
		player.seek(5);
		expect(player.position).toBe(5);
		output.elapse(1);
		expect(player.position).toBe(6);
	});

	it('gives every track its own reload', async () => {
		const { player, output } = setup();
		player.play(album(3));
		await settle();
		output.elapse(40);
		output.fail();
		await settle();
		player.next();
		await settle();
		output.elapse(40);
		output.fail();
		await settle();
		expect(player.current?.id).toBe('t1');
		expect(output.position).toBe(40);
	});

	it('leaves a lost session to the door rather than reloading', async () => {
		const { player, output } = setup({ me: () => Promise.reject(new UnauthorizedError()) });
		player.play(album(2));
		await settle();
		output.elapse(40);
		output.fail();
		await settle();
		expect(player.status).toBe('paused');
		expect(player.position).toBe(40);
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

	describe('plays', () => {
		it('reports a track heard for half its length, once, with when it started', async () => {
			vi.useFakeTimers({ now: 5000, toFake: ['Date'] });
			const { player, output, reported } = setup();
			player.play(album(2));
			vi.useRealTimers();
			await settle();
			output.elapse(49);
			expect(reported).not.toHaveBeenCalled();
			output.elapse(1);
			output.elapse(10);
			expect(reported.mock.calls).toEqual([['t0', 5000]]);
		});

		it('reports each track of a queue for itself', async () => {
			const { player, output, reported } = setup();
			player.play(album(2));
			await settle();
			output.elapse(100);
			await settle();
			output.elapse(100);
			expect(reported.mock.calls.map(([id]) => id)).toEqual(['t0', 't1']);
		});

		it('earns nothing by seeking to the end', async () => {
			const { player, output, reported } = setup();
			player.play(album(1));
			await settle();
			output.elapse(5);
			player.seek(100);
			output.elapse(1);
			await settle();
			expect(player.finished).toBe(true);
			expect(reported).not.toHaveBeenCalled();
		});

		it('earns nothing from a run of small seeks', async () => {
			const { player, output, reported } = setup();
			player.play(album(1));
			await settle();
			for (let seconds = 0; seconds < 95; seconds += 5) {
				player.seek(seconds);
			}
			output.elapse(1);
			expect(reported).not.toHaveBeenCalled();
		});

		it('counts a one-track album replayed from the end twice', async () => {
			const { player, output, reported } = setup();
			player.play(album(1));
			await settle();
			output.elapse(100);
			await settle();
			expect(player.finished).toBe(true);
			player.toggle();
			await settle();
			output.elapse(49);
			expect(reported).toHaveBeenCalledOnce();
			output.elapse(1);
			expect(reported).toHaveBeenCalledTimes(2);
		});

		it('starts a new listen on a restart from Previous', async () => {
			const { player, output, reported } = setup();
			player.play(album(1));
			await settle();
			output.elapse(60);
			player.previous();
			output.elapse(49);
			expect(reported).toHaveBeenCalledOnce();
			output.elapse(1);
			expect(reported).toHaveBeenCalledTimes(2);
		});

		it('carries on the same listen through a pause', async () => {
			const { player, output, reported } = setup();
			player.play(album(1));
			await settle();
			output.elapse(30);
			player.toggle();
			await settle();
			player.toggle();
			await settle();
			output.elapse(19);
			expect(reported).not.toHaveBeenCalled();
			output.elapse(1);
			expect(reported).toHaveBeenCalledOnce();
		});

		it('credits nothing for the way back from a reload after a retag', async () => {
			const { player, output, reported } = setup();
			player.play(album(1));
			await settle();
			output.elapse(40);
			output.fail();
			await settle();
			expect(output.position).toBe(40);
			output.seek(0); // The element starting over, before its seek lands.
			output.seek(40);
			output.elapse(9);
			expect(reported).not.toHaveBeenCalled();
			output.elapse(1);
			expect(reported).toHaveBeenCalledOnce();
		});

		it('carries on the same listen when a stale track is played again', async () => {
			const { player, output, reported } = setup();
			player.play(album(1));
			await settle();
			output.elapse(40);
			output.stopFromOutside();
			output.fail();
			await settle();
			player.toggle();
			await settle();
			output.elapse(9);
			expect(reported).not.toHaveBeenCalled();
			output.elapse(1);
			expect(reported).toHaveBeenCalledOnce();
		});

		it('never counts a track under 30 seconds', async () => {
			const { player, output, reported } = setup();
			player.play(album(1, 'a', { durationMs: 29_000 }));
			await settle();
			output.elapse(29);
			await settle();
			expect(reported).not.toHaveBeenCalled();
		});

		it('forgets the listen at a reset', async () => {
			const { player, output, reported } = setup();
			player.play(album(1));
			await settle();
			output.elapse(40);
			player.reset();
			output.seek(90);
			expect(reported).not.toHaveBeenCalled();
		});
	});
});

describe('listens', () => {
	/** Each report the library was given: what, how it ended, and how much was heard. */
	function listening() {
		const kit = setup();
		const reports = vi.spyOn(kit.library, 'reportListen').mockImplementation(() => undefined);
		const said = () =>
			reports.mock.calls.map(([report]) =>
				[report.trackId, report.ended ?? 'start', report.listenedMs ?? 0, report.shuffle].join(' ')
			);
		return { ...kit, reports, said };
	}

	it('reports the start when the sound comes, and completed at the end', async () => {
		const { player, output, said } = listening();
		player.play(album(2));
		await settle();
		expect(said()).toEqual(['t0 start 0 ']);
		output.elapse(100);
		await settle();
		expect(said()).toEqual(['t0 start 0 ', 't0 completed 100000 ', 't1 start 0 ']);
	});

	it('calls Next while it sounds a skip, with how much was heard', async () => {
		const { player, output, said, reports } = listening();
		player.play(album(3));
		await settle();
		output.elapse(12);
		player.next();
		expect(said()).toEqual(['t0 start 0 ', 't0 skipped 12000 ', 't1 start 0 ']);
		const [start] = reports.mock.calls.at(0) ?? [];
		const [end] = reports.mock.calls.at(1) ?? [];
		expect(end?.clientId).toBe(start?.clientId);
	});

	it('calls choosing another song a skip', async () => {
		const { player, output, said } = listening();
		player.play(album(3));
		await settle();
		output.elapse(3);
		player.play(album(2, 'b'));
		expect(said()).toContain('t0 skipped 3000 ');
	});

	it('never calls pausing, a restart, closing or an error a skip', async () => {
		const { player, output, said } = listening();
		player.play(album(3));
		await settle();
		output.elapse(5);
		player.pause();
		player.next();
		await settle();
		output.elapse(10);
		player.previous();
		await settle();
		output.elapse(2);
		player.leave();
		expect(said()).toEqual([
			't0 start 0 ',
			't0 stopped 5000 ',
			't1 start 0 ',
			't1 stopped 10000 ',
			't1 start 0 ',
			't1 interrupted 2000 '
		]);
	});

	it('says nothing of a song that never sounded', async () => {
		const { player, output, said } = listening();
		output.answers.push(refusal('NotSupportedError'));
		player.play(album(3));
		player.next();
		await settle();
		expect(said()).toEqual(['t1 start 0 ']);
	});

	it('names the shuffle a song plays in', async () => {
		const { player, said } = listening();
		const songs = album(2).tracks.map((track) => ({ album: album(2), track }));
		player.playSongs(songs, { id: 'S1' });
		await settle();
		expect(said()).toEqual(['t0 start 0 S1']);
	});
});
