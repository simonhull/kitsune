import { cueOf, type NowPlaying } from '#lib/library/cue.ts';
import { library, type Library, type PlayerView } from '#lib/library/library.svelte.ts';
import type { Album, ListenEnding, Track } from '#lib/library/source.ts';
import { playedGain } from './gain.ts';
import { Listen } from './listen.ts';
import { playbackSettings } from './playback.svelte.ts';
import { HtmlOutput, mediaTypeOf, SilentOutput, type AudioOutput } from './output.ts';
import {
	albumOf,
	albumRun,
	appended,
	continued,
	currentOf,
	holds,
	inOrder,
	isShuffling,
	listed,
	next,
	onDisc,
	onwardOf,
	pendingAlbum,
	picked,
	playingNext,
	playsAlbum,
	previous,
	retagged,
	rewound,
	shuffled,
	shuffledOff,
	shuffledOn,
	takenIn,
	without,
	reordered,
	throughList,
	upcoming,
	extended,
	withdrawn,
	type AlbumLookup,
	type ListEntry,
	type Onward,
	type Queue,
	type Queued,
	type ShuffleRef,
	type StartedFrom
} from './queue.ts';
import { cycled, repeatIn, type Repeat } from './repeat.ts';

export type PlayerStatus = 'idle' | 'loading' | 'playing' | 'paused';

/**
 * What the player tells whoever remembers where it is: a song first sounding, a pause, a seek, or
 * a change of how it goes on (Shuffle or Repeat).
 */
export type Heard = 'sounded' | 'paused' | 'seeked' | 'changed';

/** A fresh seed for the player's own shuffle: 32 bits, what the generator takes. */
export function newSeed(): number {
	return Math.floor(Math.random() * 2 ** 32);
}

/** How many tracks may fail in a row before the player stops trying. */
const mostFailures = 3;
/** How far before the end the next track starts loading. */
const preloadSeconds = 15;
/**
 * How long after a song from a list queue begins the next song's album is fetched: past its first
 * sound, so the fetch never shares the play path, and long before the song's end.
 */
export const listPrefetchMs = 1000;
/** Past this far into a track, Previous means the start of it. */
const restartSeconds = 3;
/** How far a track must play on from a reload for the file to count as sound again. */
const recoveredSeconds = 5;

/** What's playing, whatever the page is doing. Sound never waits for a picture. */
export class Player implements PlayerView {
	queue = $state.raw<Queue | null>(null);
	current = $derived<Track | null>(this.queue === null ? null : (currentOf(this.queue) ?? null));
	status = $state<PlayerStatus>('idle');
	/** Seconds into {@link Player.current}. */
	position = $state(0);
	/** Set when it stopped because tracks wouldn't play. */
	failure = $state(false);
	/** Set when it came to rest at the end of the queue. */
	finished = $state(false);
	/**
	 * Repeat as set: it outlives a queue, as a mode of the player. What it does now is
	 * {@link Player.repeating}.
	 */
	repeat = $state<Repeat>('off');
	/**
	 * Shows a track change on the page, when the page is ready. The sound doesn't wait for this. Set
	 * by the shell, which knows what's in the air.
	 */
	carry: (apply: () => void) => void = (apply) => {
		apply();
	};
	/**
	 * Told each time a track begins, by what the queue goes on to ({@link onwardOf}): what keeps a
	 * shuffle's queue, or a list's, ahead of it. Each is set by its owner.
	 */
	readonly onward: Partial<Record<Onward, (queue: Queue) => void>> = {};
	/**
	 * Told when a song first sounds, when it pauses, for a person or not, and when it seeks: what
	 * saving where it is hangs off. Set by its owner. It must only note the moment and return: it
	 * runs on the play path.
	 */
	heard: (moment: Heard) => void = () => undefined;

	readonly #library: Library;
	readonly #untagged: () => number;
	readonly #create: () => AudioOutput;
	#built: AudioOutput | null = null;
	/** Moves on with every new intent: anything older that answers late is ignored. */
	#run = 0;
	/** The last run sent to find out why it failed: an error event and a rejection are one failure. */
	#asked = -1;
	#failures = 0;
	/** The output no longer holds this track as it should: the next play loads it again. */
	#stale = false;
	/** The track already sent to preload. */
	#ahead: string | null = null;
	/** Where the track was reloaded to, until it has played on from there. */
	#reloadedAt: number | null = null;
	/** The listen under way. Nothing is, from the end of the queue until it goes round again. */
	#listen: Listen | null = null;
	/** The listen Last.fm was last told about, as now playing. */
	#announced: Listen | null = null;
	/** Set by a restore: the first sound does what beginning the queue would have. */
	#restored = false;

	/**
	 * Nothing audio-related exists until the first play: this module loads with no window.
	 * `untagged` is the gain, in dB, of a song with no ReplayGain, read as each song loads.
	 */
	constructor(host: Library, create: () => AudioOutput, untagged: () => number = () => 0) {
		this.#library = host;
		this.#create = create;
		this.#untagged = untagged;
		host.follow(this);
	}

	/** Reports whether sound is coming, or on its way. */
	get sounding(): boolean {
		return this.status === 'playing' || this.status === 'loading';
	}

	/** Reports whether a queue has started. It stays started, paused or finished, until a reset. */
	get started(): boolean {
		return this.queue !== null;
	}

	/** Whether the queue plays an endless shuffle, which never comes to an end. */
	get endless(): boolean {
		return this.queue?.shuffle?.endless === true;
	}

	/** What Repeat does now: `all` is off in an endless shuffle. */
	get repeating(): Repeat {
		return repeatIn(this.repeat, this.endless);
	}

	/** Whether Shuffle is lit: the player's own shuffle, or a shuffle session's queue. */
	get shuffling(): boolean {
		return this.queue !== null && isShuffling(this.queue);
	}

	/** Moves Repeat on: off, all, this song, off. An endless shuffle skips all. */
	cycleRepeat(): void {
		this.repeat = cycled(this.repeat, this.endless);
		// The song preloaded may no longer be the one that comes next.
		this.#ahead = null;
		const { queue } = this;
		if (queue !== null) {
			// A list may go round now, or no longer: it looks again at what comes after.
			this.onward[onwardOf(queue)]?.(queue);
			this.heard('changed');
		}
	}

	/**
	 * Turns the player's own Shuffle on or off. On, the songs after the one playing take an order
	 * from `seed`; off, they go back to the queue's own order from the song playing. The song
	 * playing never stops, and nothing waits: the save follows. A shuffle session's queue is the
	 * shuffler's to leave ({@link Player.leaveShuffle}).
	 */
	toggleShuffle(seed: number = newSeed()): void {
		const { queue } = this;
		if (queue === null || queue.shuffle !== undefined) {
			return;
		}
		this.queue =
			queue.shuffled === undefined
				? shuffledOn(queue, seed)
				: takenIn(shuffledOff(queue), this.#lookup);
		this.#reordered();
	}

	/**
	 * Leaves a shuffle session: the song playing plays on, and `later` comes after it, as a list's
	 * songs do. Without `later`, which the scope's order may not have yet, the songs already queued
	 * stay until it comes.
	 */
	leaveShuffle(later?: readonly ListEntry[]): void {
		const { queue } = this;
		// The player's own shuffle isn't a session's; with neither and no order, nothing changes.
		if (
			queue === null ||
			queue.shuffled !== undefined ||
			(queue.shuffle === undefined && later === undefined)
		) {
			return;
		}
		const left: Queue = { ...queue, shuffle: undefined };
		this.queue =
			later === undefined
				? left
				: takenIn(
						{
							...left,
							tracks: queue.tracks.slice(0, queue.index + 1),
							later: later.length > 0 ? later : undefined
						},
						this.#lookup
					);
		this.#reordered();
	}

	/** The songs to come have changed under the one playing: show it, preload again, save it later. */
	#reordered(): void {
		this.#ahead = null;
		this.#show();
		this.heard('changed');
	}

	/** What the island and Now playing should name: the player owns the order. */
	get cue(): NowPlaying | null {
		const { queue } = this;
		if (queue === null) {
			return null;
		}
		const cue = cueOf(queue.album, queue.tracks, queue.index, queue.albums);
		if (cue === null) {
			return null;
		}
		// A list queue holds its songs ahead as names: the place is the song's in the whole list.
		const later = queue.later?.length ?? 0;
		if (later > 0) {
			return { ...cue, trackCount: cue.trackCount + later, moreCount: cue.moreCount + later };
		}
		// Going on through a list, the queue grows an album at a time: the place that means
		// something is the song's in its album.
		return queue.continues === undefined ? cue : { ...cue, ...albumRun(queue) };
	}

	/** The album of the song playing, which in a queue of picked songs changes as it goes. */
	get album(): Album | null {
		const { queue, current } = this;
		return queue === null || current === null ? null : albumOf(queue, current);
	}

	get #output(): AudioOutput {
		if (this.#built === null) {
			this.#built = this.#create();
			this.#built.listen({
				time: (seconds) => {
					this.#timed(seconds);
				},
				running: () => {
					this.#sounding();
				},
				stopped: () => {
					this.#stopped();
				},
				ended: () => {
					this.#ended();
				},
				failed: () => void this.#mediaFailed(this.#run)
			});
		}
		return this.#built;
	}

	/** Plays an album in its own order, from `index`. From a list, it goes on through the list. */
	play(album: Album, index = 0, from: StartedFrom = 'elsewhere'): void {
		this.#fresh();
		this.#begin(started(inOrder(album, index), from), this.#movedOn());
	}

	/** Plays one disc of an album, in order. A disc with no songs plays nothing. */
	playDisc(album: Album, disc: number, from: StartedFrom = 'elsewhere'): void {
		const queue = onDisc(album, disc);
		if (queue.tracks.length > 0) {
			this.#fresh();
			this.#begin(started(queue, from), this.#movedOn());
		}
	}

	/** Plays songs picked from anywhere, in the order given; `shuffle` names the shuffle they start. */
	playSongs(songs: readonly Queued[], shuffle?: ShuffleRef): void {
		const queue = picked(songs);
		if (queue !== null) {
			this.#fresh();
			this.#begin(shuffle === undefined ? queue : { ...queue, shuffle }, this.#movedOn());
		}
	}

	/**
	 * Plays a list that only names its songs, from `first`, whose album is here: what plays on is
	 * `later`, in order, each song's album fetched after the song before it has begun, and the
	 * queue comes to rest after the last. Nothing waits on the server before the first sound.
	 */
	playList(first: Queued, later: readonly ListEntry[]): void {
		this.#fresh();
		this.#begin(listed(first, later, this.#lookup), this.#movedOn());
	}

	/** What a list queue knows of an album: here, never to be had, or not yet. */
	readonly #lookup: AlbumLookup = (id) => {
		const album = this.#library.album(id);
		if (album !== undefined) {
			return album;
		}
		return this.#library.failed(id) || this.#library.unavailable(id) ? 'gone' : undefined;
	};

	/**
	 * Keeps a list queue's next song ahead of it: once a song has begun, and is sounding, the album
	 * of the next song it only names is fetched, and its songs go in.
	 */
	#feedList(queue: Queue): void {
		const next = queue.later?.at(0);
		if (next === undefined) {
			return;
		}
		setTimeout(() => {
			void this.#library.loadAlbum(next.albumId).then(() => {
				this.#takeInList();
			});
		}, listPrefetchMs);
	}

	/** Takes into the queue playing whatever of its list has its albums here now. */
	#takeInList(): void {
		const { queue } = this;
		if (queue?.later === undefined) {
			return;
		}
		const grown = takenIn(queue, this.#lookup);
		if (grown !== queue) {
			this.queue = grown;
			this.#show();
		}
	}

	/**
	 * A list queue whose next song's album hasn't come by the end of the song before it: the one
	 * case anything waits, and only for that album.
	 */
	#awaitList(queue: Queue, ending: ListenEnding): void {
		const next = queue.later?.at(0);
		if (next === undefined) {
			this.#finish(queue, ending);
			return;
		}
		this.#run += 1;
		const run = this.#run;
		this.status = 'loading';
		void this.#library.loadAlbum(next.albumId).then(() => {
			if (run !== this.#run || this.queue !== queue) {
				return;
			}
			this.queue = takenIn(queue, this.#lookup);
			this.#advance(ending);
		});
	}

	/** Puts songs after the one playing; with nothing started, plays them. */
	playNext(songs: readonly Queued[]): void {
		if (this.queue === null) {
			this.playSongs(songs);
		} else {
			this.queue = playingNext(this.queue, songs);
			this.#ahead = null;
			this.#show();
		}
	}

	/** Adds a shuffle's next songs, while its queue is the one playing. Reports whether it was. */
	extendShuffle(shuffle: ShuffleRef, songs: readonly Queued[]): boolean {
		if (this.queue?.shuffle !== shuffle) {
			return false;
		}
		this.queue = extended(this.queue, songs);
		this.#show();
		return true;
	}

	/**
	 * Says which album comes after `after` in the list the queue goes on through: `null` for none,
	 * at the end of the list or with `after` gone from it. An album put after it that hasn't begun
	 * gives way to this one. Reports whether the queue still goes on from `after`.
	 */
	continueList(after: string, album: Album | null): boolean {
		let queue = this.queue;
		if (queue?.continues === undefined) {
			return false;
		}
		const ahead = pendingAlbum(queue);
		if (ahead?.after === after) {
			if (queue.continues.album === album?.id) {
				return true;
			}
			queue = withdrawn(queue);
		}
		if (queue.continues?.album !== after) {
			return false;
		}
		const onward = album === null ? queue : continued(queue, album);
		if (onward !== this.queue) {
			this.queue = onward;
			this.#show();
		}
		return true;
	}

	/** Puts songs at the end of the queue; with nothing started, plays them. */
	addToQueue(songs: readonly Queued[]): void {
		if (this.queue === null) {
			this.playSongs(songs);
		} else {
			this.queue = appended(this.queue, songs);
			this.#show();
		}
	}

	/** Takes songs out of what's still to play. */
	removeAhead(ids: readonly string[]): void {
		if (this.queue !== null) {
			this.queue = without(this.queue, ids);
			this.#ahead = null;
			this.#show();
		}
	}

	/** Puts what's still to play in a new order, by track ID. */
	reorderAhead(ahead: readonly string[]): void {
		if (this.queue !== null) {
			this.queue = reordered(this.queue, ahead);
			this.#ahead = null;
			this.#show();
		}
	}

	shuffle(album: Album): void {
		this.#fresh();
		this.#begin(shuffled(album, Math.floor(Math.random() * 2 ** 32)), this.#movedOn());
	}

	/**
	 * Plays an album from the top, or toggles it if it's the one in the player (`playsAlbum`). From
	 * a list, it goes on through the list.
	 */
	playOrToggle(album: Album, from: StartedFrom = 'elsewhere'): void {
		if (playsAlbum(this.queue, album.id, from)) {
			this.toggle();
		} else {
			this.play(album, 0, from);
		}
	}

	/** Plays or pauses. With nothing loaded, there's nothing to do. */
	toggle(): void {
		if (this.queue === null) {
			return;
		}
		if (this.sounding) {
			this.pause();
		} else {
			this.resume();
		}
	}

	pause(): void {
		this.#run += 1;
		this.status = 'paused';
		this.#built?.pause();
		this.heard('paused');
	}

	/**
	 * Comes back to where someone left off, paused: the queue, the song and how far in, with no
	 * sound, no listen and nothing loaded. Play loads the song there and sounds, as after a reload.
	 */
	restore(queue: Queue, seconds: number, repeat: Repeat = 'off'): void {
		this.#endListen('stopped');
		this.#fresh();
		this.#run += 1;
		this.queue = queue;
		this.repeat = repeat;
		const current = currentOf(queue);
		this.position = Math.min(Math.max(seconds, 0), (current?.durationMs ?? 0) / 1000);
		this.status = 'paused';
		this.#stale = true;
		this.#ahead = null;
		this.#reloadedAt = null;
		this.#listen = null;
		this.#restored = true;
		this.#show();
	}

	resume(): void {
		const { current } = this;
		if (current === null) {
			return;
		}
		this.#fresh();
		this.#run += 1;
		this.status = 'loading';
		this.#listen ??= this.#listenFromHere();
		if (this.#stale) {
			this.#stale = false;
			this.#load(current);
			if (this.position > 0) {
				this.#output.seek(this.position);
			}
		}
		void this.#sound(this.#run);
	}

	next(): void {
		if (this.queue !== null) {
			this.#fresh();
			this.#advance(this.#movedOn());
		}
	}

	/** Skips on to a song further along the queue, by its place in it: the songs passed are skipped. */
	skipTo(index: number): void {
		if (this.queue === null || index <= this.queue.index || index >= this.queue.tracks.length) {
			return;
		}
		this.#fresh();
		this.#begin({ ...this.queue, index }, this.#movedOn());
	}

	/** Goes back to the start of the track, or past that to the one before. */
	previous(): void {
		if (this.queue === null) {
			return;
		}
		this.#fresh();
		const back = previous(this.queue);
		if (back === null || this.position > restartSeconds) {
			this.seek(0);
			// Starting a song again is no verdict on it, and a listen of its own.
			this.#endListen('stopped');
			const listen = this.#listenFromHere();
			this.#listen = listen;
			if (listen !== null && this.status === 'playing') {
				this.#announced = listen;
				listen.sounded = true;
				this.#library.reportListen(listen.report());
			}
		} else {
			this.#begin(back, this.#movedOn());
		}
	}

	seek(seconds: number): void {
		const { current } = this;
		if (current === null) {
			return;
		}
		this.position = Math.min(Math.max(seconds, 0), current.durationMs / 1000);
		this.#reloadedAt = null;
		this.#listen?.jumped(this.position);
		if (!this.#stale) {
			this.#output.seek(this.position);
		}
		this.heard('seeked');
	}

	/** Takes in an album just edited: the names move at once, and the sound doesn't notice. */
	retag(album: Album): void {
		if (this.queue !== null && holds(this.queue, album.id)) {
			this.queue = retagged(this.queue, album);
		}
	}

	/** Stops and forgets the queue, so the next user can't inherit it. The output stays built. */
	reset(): void {
		this.#endListen('stopped');
		this.#run += 1;
		this.#built?.stop();
		this.queue = null;
		this.status = 'idle';
		this.position = 0;
		this.repeat = 'off';
		this.#fresh();
		this.#stale = false;
		this.#ahead = null;
		this.#reloadedAt = null;
		this.#listen = null;
		this.#restored = false;
	}

	/** The page is going away, and the song with it: that's no verdict on the song either. */
	leave(): void {
		this.#endListen('interrupted');
	}

	/**
	 * How the song playing ends when the person moves on from it: a skip while it sounds. Moving on
	 * from a paused song is not, because pausing is never a skip.
	 */
	#movedOn(): ListenEnding {
		return this.sounding ? 'skipped' : 'stopped';
	}

	/** Reports how the listen under way ended, if it ever sounded, and lets it go. */
	#endListen(ending: ListenEnding): void {
		const listen = this.#listen;
		this.#listen = null;
		if (listen?.sounded === true) {
			this.#library.reportListen(listen.report(ending));
		}
	}

	/** Someone asked for this, so whatever went wrong before is behind us. */
	#fresh(): void {
		this.#failures = 0;
		this.failure = false;
		this.finished = false;
	}

	#begin(queue: Queue, ending: ListenEnding): void {
		this.#endListen(ending);
		this.#run += 1;
		const run = this.#run;
		this.queue = queue;
		this.position = 0;
		this.status = 'loading';
		this.#stale = false;
		this.#ahead = null;
		this.#reloadedAt = null;
		this.#restored = false;
		this.#listen = this.#listenFromHere();
		this.#show();
		this.onward[onwardOf(queue)]?.(queue);
		this.#feedList(queue);
		const track = currentOf(queue);
		if (track === undefined) {
			return;
		}
		if (this.#playable(track)) {
			this.#load(track);
			void this.#sound(run);
		} else {
			this.#trackFailed();
		}
	}

	#listenFromHere(): Listen | null {
		const { current, position } = this;
		return current === null ? null : new Listen(current, Date.now(), position, this.queue?.shuffle);
	}

	/**
	 * Moves the names on. With nothing showing, the island arrives at once, in the press's own
	 * frame; a queue that starts on the track already showing has nothing to carry.
	 */
	#show(): void {
		const show = (): void => {
			this.#library.showQueue();
		};
		const showing = this.#library.playing;
		if (showing === null || showing.track.id === this.current?.id) {
			show();
		} else {
			this.carry(show);
		}
	}

	#load(track: Track): void {
		const output = this.#output;
		output.load(this.#source(track));
		output.setGain(playedGain(track, this.queue?.ordered ?? true, this.#untagged()));
	}

	#source({ id, durationMs }: Track): { url: string; seconds: number } {
		return { url: this.#library.streamUrl(id), seconds: durationMs / 1000 };
	}

	#playable({ format }: Track): boolean {
		const mediaType = mediaTypeOf(format);
		return mediaType === undefined || this.#output.canPlay(mediaType);
	}

	async #sound(run: number): Promise<void> {
		try {
			await this.#output.play();
			if (run === this.#run) {
				this.#sounding();
			}
		} catch (error) {
			if (run !== this.#run) {
				return;
			}
			const name = error instanceof Error ? error.name : '';
			if (name === 'NotAllowedError') {
				this.#hold(false);
			} else if (name !== 'AbortError') {
				await this.#mediaFailed(run);
			}
		}
	}

	#sounding(): void {
		if (this.status === 'loading') {
			this.status = 'playing';
			this.#failures = 0;
		}
		if (this.#listen !== null && this.#announced !== this.#listen) {
			// Once per listen, when the sound first comes: a resume is the same song still playing.
			this.#announced = this.#listen;
			this.#listen.sounded = true;
			this.#library.nowPlaying(this.#listen.trackId);
			this.#library.reportListen(this.#listen.report());
			this.heard('sounded');
		}
		const { queue } = this;
		if (this.#restored && queue !== null) {
			// What beginning it would have done: a shuffle tops up, a list fetches its next album.
			this.#restored = false;
			this.onward[onwardOf(queue)]?.(queue);
			this.#feedList(queue);
		}
	}

	#stopped(): void {
		if (this.sounding) {
			this.#hold(false);
		}
	}

	/** Pauses here, for a reason that isn't a person pressing pause. */
	#hold(stale: boolean): void {
		this.#run += 1;
		this.status = 'paused';
		this.#stale ||= stale;
		this.heard('paused');
	}

	#timed(seconds: number): void {
		if (this.#reloadedAt !== null) {
			// A reload starts from nothing, and this is it on its way back.
			if (seconds < this.#reloadedAt) {
				return;
			}
			if (seconds >= this.#reloadedAt + recoveredSeconds) {
				this.#reloadedAt = null;
			}
		}
		this.position = seconds;
		if (this.#listen?.heard(seconds) === true) {
			this.#library.reportPlay(this.#listen.trackId, this.#listen.startedAt);
		}
		const { queue, current } = this;
		// The same song again is already here: nothing to preload.
		const following =
			queue === null || this.repeating === 'one' ? undefined : this.#following(queue);
		if (
			current !== null &&
			following !== undefined &&
			following.id !== this.#ahead &&
			current.durationMs / 1000 - seconds <= preloadSeconds &&
			this.#playable(following)
		) {
			this.#ahead = following.id;
			this.#output.preload(this.#source(following));
		}
	}

	/** A song played to its end: with Repeat on this song it plays again from the start, as a new listen. */
	#ended(): void {
		const { queue } = this;
		if (queue !== null && this.repeating === 'one') {
			this.#begin({ ...queue }, 'completed');
			return;
		}
		this.#advance('completed');
	}

	/** The song that plays after this one: the next, or with Repeat on all, the first again. */
	#following(queue: Queue): Track | undefined {
		const after = upcoming(queue);
		const goesRound =
			this.repeating === 'all' && queue.later === undefined && queue.continues === undefined;
		return after ?? (goesRound ? queue.tracks.at(0) : undefined);
	}

	#advance(ending: ListenEnding): void {
		if (this.queue === null) {
			return;
		}
		const onward = next(this.queue);
		if (onward !== null) {
			this.#begin(onward, ending);
			return;
		}
		// A list queue takes in what has come for it; what hasn't, it waits for.
		const grown = takenIn(this.queue, this.#lookup);
		const after = next(grown);
		if (after !== null) {
			this.#begin(after, ending);
		} else if (grown.later === undefined && this.repeating === 'all') {
			// Round again from the first song, in the order it plays: nothing past it comes back.
			this.#begin(rewound(grown), ending);
		} else if (grown.later === undefined) {
			this.#finish(grown, ending);
		} else {
			this.#awaitList(grown, ending);
		}
	}

	/** The end of the queue: back at the first track, paused, ready to go round again. */
	#finish(queue: Queue, ending: ListenEnding): void {
		this.#endListen(ending);
		this.#built?.pause();
		this.queue = rewound(queue);
		this.position = 0;
		this.finished = true;
		this.#hold(true);
		this.#show();
	}

	/** Finds out what a media failure was: the element can't tell a lost session from a bad file. */
	async #mediaFailed(run: number): Promise<void> {
		if (this.#asked === run) {
			return;
		}
		if (this.status === 'paused') {
			this.#stale = true;
			return;
		}
		this.#asked = run;
		const session = await this.#library.askSession();
		if (run !== this.#run) {
			return;
		}
		if (session !== 'signed-in') {
			// A lost session has gone to the door. A server restarting must not burn the queue.
			this.#hold(true);
		} else if (!this.#reload()) {
			this.#trackFailed();
		}
	}

	/**
	 * Loads a track that was playing again, where it was. A retag replaces the file under the element,
	 * and the media dies at its next range request, though the track is fine. A file that has really
	 * gone fails this too, which is how that gets found out. Reports whether it tried: a track that
	 * never got going isn't a file that changed, and one reload is all a track gets until it has
	 * played on.
	 */
	#reload(): boolean {
		const { current, position } = this;
		if (current === null || position === 0 || this.#reloadedAt !== null) {
			return false;
		}
		this.#run += 1;
		this.status = 'loading';
		this.#reloadedAt = position;
		this.#listen?.jumped(position);
		this.#load(current);
		this.#output.seek(position);
		void this.#sound(this.#run);
		return true;
	}

	/** Skips a track that won't play, unless that's enough of them or there's nothing left. */
	#trackFailed(): void {
		this.#failures += 1;
		const onward = this.queue === null ? null : next(this.queue);
		if (onward === null || this.#failures >= mostFailures) {
			this.#endListen('interrupted');
			this.failure = true;
			this.#hold(true);
		} else {
			this.#begin(onward, 'interrupted');
		}
	}
}

/** Marks a queue started from a list, which goes on through it. */
function started(queue: Queue, from: StartedFrom): Queue {
	return from === 'list' ? throughList(queue) : queue;
}

/** The one player. The fixtures have nothing to hear, so they get the clock and no sound. */
export const player = new Player(
	library,
	() => (library.simulated ? new SilentOutput() : new HtmlOutput()),
	() => playbackSettings.untaggedGain
);
