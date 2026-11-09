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
	next,
	onDisc,
	onwardOf,
	pendingAlbum,
	picked,
	playingNext,
	previous,
	retagged,
	rewound,
	shuffled,
	throughList,
	upcoming,
	extended,
	withdrawn,
	type Onward,
	type Queue,
	type Queued,
	type ShuffleRef
} from './queue.ts';

export type PlayerStatus = 'idle' | 'loading' | 'playing' | 'paused';

/** Where a queue was started: a list goes on past the album, anywhere else comes to rest after it. */
export type StartedFrom = 'list' | 'elsewhere';

/** How many tracks may fail in a row before the player stops trying. */
const mostFailures = 3;
/** How far before the end the next track starts loading. */
const preloadSeconds = 15;
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

	/** What the island and Now playing should name: the player owns the order. */
	get cue(): NowPlaying | null {
		const { queue } = this;
		if (queue === null) {
			return null;
		}
		const cue = cueOf(queue.album, queue.tracks, queue.index, queue.albums);
		// Going on through a list, the queue grows an album at a time: the place that means
		// something is the song's in its album.
		return cue === null || queue.continues === undefined ? cue : { ...cue, ...albumRun(queue) };
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
					this.#advance('completed');
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

	shuffle(album: Album): void {
		this.#fresh();
		this.#begin(shuffled(album, Math.floor(Math.random() * 2 ** 32)), this.#movedOn());
	}

	/** Plays an album from the top, or toggles it if it's the one in the player. */
	playOrToggle(album: Album): void {
		if (this.queue?.album.id === album.id && this.queue.albums === undefined) {
			this.toggle();
		} else {
			this.play(album);
		}
	}

	/** Plays or pauses. The first press plays what's cued. */
	toggle(): void {
		if (this.#startCue(0)) {
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
		if (!this.#startCue(1)) {
			this.#fresh();
			this.#advance(this.#movedOn());
		}
	}

	/** Goes back to the start of the track, or past that to the one before. */
	previous(): void {
		if (this.#startCue(0) || this.queue === null) {
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
		this.#fresh();
		this.#stale = false;
		this.#ahead = null;
		this.#reloadedAt = null;
		this.#listen = null;
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

	/** With nothing started, the transport means what's cued, `ahead` tracks in. */
	#startCue(ahead: number): boolean {
		const cue = this.#library.playing;
		if (this.queue !== null || cue === null) {
			return false;
		}
		this.play(cue.album, Math.min(cue.index + ahead, cue.trackCount - 1));
		return true;
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
		this.#listen = this.#listenFromHere();
		this.#show();
		this.onward[onwardOf(queue)]?.(queue);
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

	/** Moves the names on. A queue that starts on the track already cued has nothing to carry. */
	#show(): void {
		const show = (): void => {
			this.#library.showQueue();
		};
		if (this.#library.playing?.track.id === this.current?.id) {
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
		const following = queue === null ? undefined : upcoming(queue);
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

	#advance(ending: ListenEnding): void {
		if (this.queue === null) {
			return;
		}
		const onward = next(this.queue);
		if (onward === null) {
			this.#finish(this.queue, ending);
		} else {
			this.#begin(onward, ending);
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
