/**
 * The signed-in person's playlists, as every page sees them: the list, each one read as it's opened,
 * and what they hold of songs being added. Every change is optimistic: the copy here changes at
 * once, and the save follows, one at a time for each playlist, so a move can't land before the
 * move before it. A `playlists` event reads what it names again, once nothing of this browser's is
 * still on its way; an answer that comes after a change made here is let go.
 */
import { SvelteMap } from 'svelte/reactivity';
import {
	distinct,
	movedTo,
	nowIso,
	putBackInto,
	recount,
	summaryOf,
	takenFrom,
	type TakenOut
} from './playlistEdits.ts';
import type { LibrarySource, PlaylistAdded, PlaylistDetail, PlaylistSummary } from './source.ts';

export type { TakenOut } from './playlistEdits.ts';

/** Where something asked for has got to, as {@link Library} has it. */
type Load<T> =
	{ kind: 'loading' } | { kind: 'ready'; value: T } | { kind: 'missing' } | { kind: 'unavailable' };

/** The most songs `holding` asks about: past it, nothing says what a playlist has already. */
export const mostHolding = 1000;

/** What a 404 looks like from either source. */
function isMissing(error: unknown): boolean {
	return typeof error === 'object' && error !== null && 'status' in error && error.status === 404;
}

export class Playlists {
	/** Every playlist, by name, once read. */
	list = $state.raw<Load<PlaylistSummary[]>>({ kind: 'loading' });
	readonly #details = new SvelteMap<string, Load<PlaylistDetail>>();
	/** What the songs last asked about are held by: playlist IDs to how many of them each holds. */
	readonly #holds = new SvelteMap<string, number>();
	/** The songs asked about, joined: reactive, so a menu drawn before the question sees the answer. */
	#holdsKey = $state('');
	readonly #source: LibrarySource;
	/** Each playlist's saves, one after another. */
	readonly #saving = new SvelteMap<string, Promise<unknown>>();
	/** How many saves each playlist has on their way. */
	readonly #pending = new SvelteMap<string, number>();
	/** Moves on at every change made here: a read begun before one is let go. */
	readonly #changes = new SvelteMap<string, number>();
	#listChanges = 0;
	#epoch = 0;
	#made = 0;

	constructor(source: LibrarySource) {
		this.#source = source;
	}

	/** The playlists, or none while they're not here. */
	get all(): readonly PlaylistSummary[] {
		return this.list.kind === 'ready' ? this.list.value : [];
	}

	/** One playlist as listed, if it is. */
	summary(id: string): PlaylistSummary | undefined {
		return this.all.find((p) => p.id === id);
	}

	/** Reads the list. A failure leaves what was there, or says it's unavailable. */
	async load(): Promise<void> {
		const epoch = this.#epoch;
		const changes = this.#listChanges;
		try {
			const value = await this.#source.playlists();
			if (epoch === this.#epoch && changes === this.#listChanges) {
				this.list = { kind: 'ready', value: this.#withPending(value) };
			}
		} catch {
			if (epoch === this.#epoch && this.list.kind !== 'ready') {
				this.list = { kind: 'unavailable' };
			}
		}
	}

	/** Keeps the playlists still being made here in a list read before they were. */
	#withPending(value: PlaylistSummary[]): PlaylistSummary[] {
		const pending = this.all.filter(({ id }) => id.startsWith('pending:'));
		return [...value, ...pending];
	}

	/** Where a playlist's page has got to. */
	detail(id: string): Load<PlaylistDetail> {
		return this.#details.get(id) ?? { kind: 'loading' };
	}

	/** Reads a playlist's songs, unless they're here; `again` reads them anyway. */
	async loadDetail(id: string, again = false): Promise<void> {
		const held = this.#details.get(id);
		if (!again && held?.kind === 'ready') {
			return;
		}
		const epoch = this.#epoch;
		const changes = this.#changes.get(id) ?? 0;
		try {
			const value = await this.#source.playlist(id);
			if (epoch === this.#epoch && changes === (this.#changes.get(id) ?? 0)) {
				this.#details.set(id, { kind: 'ready', value });
				this.#setSummary(summaryOf(value));
			}
		} catch (error) {
			if (epoch === this.#epoch && this.#details.get(id)?.kind !== 'ready') {
				this.#details.set(id, { kind: isMissing(error) ? 'missing' : 'unavailable' });
			}
		}
	}

	/**
	 * Asks what the playlists hold of these songs, for Add to playlist's marks. A question already
	 * asked isn't asked again; one about too many songs isn't asked.
	 */
	async askHolds(trackIds: readonly string[]): Promise<void> {
		const key = trackIds.join(',');
		if (key === this.#holdsKey || trackIds.length === 0 || trackIds.length > mostHolding) {
			return;
		}
		this.#holdsKey = key;
		this.#holds.clear();
		const epoch = this.#epoch;
		try {
			const answer = await this.#source.playlists(trackIds);
			if (epoch === this.#epoch && key === this.#holdsKey) {
				for (const p of answer) {
					this.#holds.set(p.id, p.holds ?? 0);
				}
			}
		} catch {
			// Nothing says what they hold: the menu shows them all as they are.
		}
	}

	/** How many of the songs last asked about the playlist holds, or undefined while it's not known. */
	holds(playlistId: string, trackIds: readonly string[]): number | undefined {
		return trackIds.join(',') === this.#holdsKey ? this.#holds.get(playlistId) : undefined;
	}

	/**
	 * Makes a playlist with these songs. It's listed at once, as pending, and takes its own ID when
	 * the server answers. Rejects when the server won't make it, and it's gone again.
	 */
	async create(name: string, trackIds: readonly string[] = []): Promise<PlaylistAdded> {
		this.#made += 1;
		const temp = `pending:${String(this.#made)}`;
		const now = nowIso();
		this.#listChanges += 1;
		this.#setList([
			...this.all,
			{
				id: temp,
				name,
				source: 'kitsune',
				songCount: trackIds.length,
				albumCount: 0,
				durationMs: 0,
				mosaic: [],
				createdAt: now,
				updatedAt: now,
				...(trackIds.length > 0 ? { addedAt: now } : {})
			}
		]);
		try {
			const made = await this.#source.createPlaylist(name, trackIds);
			this.#listChanges += 1;
			if (this.list.kind === 'ready') {
				this.#setList(this.all.map((p) => (p.id === temp ? made.playlist : p)));
			} else {
				void this.load();
			}
			if (trackIds.length > 0) {
				this.#heldAll(made.playlist.id, trackIds);
			}
			return made;
		} catch (error) {
			this.#listChanges += 1;
			this.#setList(this.all.filter((p) => p.id !== temp));
			throw error;
		}
	}

	/** Renames a playlist here at once, and on the server after. A refusal puts the old name back. */
	rename(id: string, name: string): Promise<void> {
		const before = this.summary(id)?.name;
		this.#change(id, (p) => ({ ...p, name }));
		return this.#save(id, async () => {
			try {
				const renamed = await this.#source.renamePlaylist(id, name);
				this.#setSummary(renamed);
			} catch (error) {
				if (before !== undefined) {
					this.#change(id, (p) => ({ ...p, name: before }));
				}
				throw error;
			}
		});
	}

	/** Deletes a playlist: gone from the list at once, back if the server says no. */
	async remove(id: string): Promise<void> {
		const before = this.all;
		this.#listChanges += 1;
		this.#setList(before.filter((p) => p.id !== id));
		try {
			await this.#save(id, () => this.#source.deletePlaylist(id));
			this.#details.delete(id);
		} catch (error) {
			if (!isMissing(error)) {
				this.#listChanges += 1;
				this.#setList(before);
				throw error;
			}
		}
	}

	/**
	 * Adds songs to a playlist. The list shows it at once, as added to now; the playlist's page, if
	 * it's been read, reads its songs again once the save is in. Answers what the server did.
	 */
	add(id: string, trackIds: readonly string[], again = false): Promise<PlaylistAdded> {
		const now = nowIso();
		const held = this.#holds.get(id) ?? 0;
		const known = trackIds.join(',') === this.#holdsKey;
		const adding = again ? trackIds.length : distinct(trackIds) - (known ? held : 0);
		this.#change(id, (p) => ({
			...p,
			songCount: p.songCount + Math.max(0, adding),
			addedAt: now,
			updatedAt: now
		}));
		return this.#save(id, async () => {
			const done = await this.#source.addToPlaylist(
				id,
				again ? { tracks: trackIds, again } : { tracks: trackIds }
			);
			this.#setSummary(done.playlist);
			this.#heldAll(id, trackIds);
			this.#stale(id);
			return done;
		});
	}

	/** Takes entries out of a playlist's page at once, and says how Undo puts them back. */
	takeOut(id: string, entryIds: readonly string[]): { taken: TakenOut; saved: Promise<string[]> } {
		const held = this.#details.get(id);
		let taken: TakenOut = { runs: [] };
		if (held?.kind === 'ready') {
			const out = takenFrom(held.value.entries, entryIds);
			taken = out.taken;
			this.#changeDetail(id, (d) => ({ ...d, entries: out.left }));
		}
		const saved = this.#save(id, () => this.#source.removeFromPlaylist(id, entryIds));
		saved.catch(() => {
			this.#stale(id);
		});
		return { taken, saved };
	}

	/** Puts back what a take-out took, each run where it was, as new entries. */
	putBack(id: string, taken: TakenOut): Promise<void> {
		this.#changeDetail(id, (d) => ({ ...d, entries: putBackInto(d.entries, taken) }));
		return this.#save(id, async () => {
			for (const run of taken.runs) {
				const added = await this.#source.addToPlaylist(id, {
					tracks: run.entries.map(({ track }) => track.id),
					again: true,
					...(run.before === undefined ? {} : { before: run.before })
				});
				const fresh = run.entries.map((entry, i) => [entry.id, added.added[i]?.id ?? entry.id]);
				this.#changeDetail(id, (d) => ({
					...d,
					entries: d.entries.map((e) => ({
						...e,
						id: fresh.find(([was]) => was === e.id)?.[1] ?? e.id
					}))
				}));
				this.#setSummary(added.playlist);
			}
		});
	}

	/**
	 * Moves an entry to a place in the page's order, at once, and saves the move: just after the
	 * entry it now follows, or before the first.
	 */
	move(id: string, entryId: string, to: number): Promise<void> {
		const held = this.#details.get(id);
		const moved = held?.kind === 'ready' ? movedTo(held.value.entries, entryId, to) : null;
		if (moved === null) {
			return Promise.resolve();
		}
		const { order, place } = moved;
		this.#changeDetail(id, (d) => ({ ...d, entries: order }));
		const saved = this.#save(id, () => this.#source.movePlaylistEntry(id, entryId, place));
		saved.catch(() => {
			this.#stale(id);
		});
		return saved;
	}

	/** The server says these playlists changed: read them again, once nothing of ours is in flight. */
	heard(ids: readonly string[]): void {
		void this.load();
		for (const id of ids) {
			if (this.#details.has(id) && (this.#pending.get(id) ?? 0) === 0) {
				void this.loadDetail(id, true);
			}
		}
	}

	/** A gap in the events: read everything held again. */
	resync(): void {
		this.heard([...this.#details.keys()]);
	}

	/** Forgets everything the last session saw. */
	reset(): void {
		this.#epoch += 1;
		this.list = { kind: 'loading' };
		this.#details.clear();
		this.#holds.clear();
		this.#holdsKey = '';
		this.#saving.clear();
		this.#pending.clear();
		this.#changes.clear();
	}

	/** Marks a playlist's page to be read again when nothing is saving: its songs changed elsewhere. */
	#stale(id: string): void {
		if (!this.#details.has(id)) {
			return;
		}
		const settled = this.#saving.get(id) ?? Promise.resolve();
		void settled.finally(() => {
			if ((this.#pending.get(id) ?? 0) === 0) {
				void this.loadDetail(id, true);
			}
		});
	}

	/** After an add, the playlist holds every one of those songs. */
	#heldAll(id: string, trackIds: readonly string[]): void {
		if (trackIds.join(',') === this.#holdsKey) {
			this.#holds.set(id, distinct(trackIds));
		}
	}

	/** Runs a save after the playlist's saves before it, counting it as pending until it settles. */
	#save<T>(id: string, run: () => Promise<T>): Promise<T> {
		const epoch = this.#epoch;
		this.#pending.set(id, (this.#pending.get(id) ?? 0) + 1);
		const before = this.#saving.get(id) ?? Promise.resolve();
		const next = before
			.catch(() => undefined)
			.then(() => (epoch === this.#epoch ? run() : Promise.reject(new Error('signed out'))))
			.finally(() => {
				this.#pending.set(id, (this.#pending.get(id) ?? 1) - 1);
			});
		this.#saving.set(id, next);
		return next;
	}

	#change(id: string, edit: (p: PlaylistSummary) => PlaylistSummary): void {
		this.#listChanges += 1;
		this.#changes.set(id, (this.#changes.get(id) ?? 0) + 1);
		this.#setList(this.all.map((p) => (p.id === id ? edit(p) : p)));
		const held = this.#details.get(id);
		if (held?.kind === 'ready') {
			this.#details.set(id, {
				kind: 'ready',
				value: { ...held.value, ...edit(summaryOf(held.value)) }
			});
		}
	}

	#changeDetail(id: string, edit: (d: PlaylistDetail) => PlaylistDetail): void {
		const held = this.#details.get(id);
		if (held?.kind !== 'ready') {
			return;
		}
		this.#changes.set(id, (this.#changes.get(id) ?? 0) + 1);
		const value = recount(edit(held.value));
		this.#details.set(id, { kind: 'ready', value });
		this.#listChanges += 1;
		this.#setList(
			this.all.map((p) => (p.id === id ? { ...p, ...summaryOf(value), name: p.name } : p))
		);
	}

	/** Takes in what the server says of one playlist. */
	#setSummary(summary: PlaylistSummary): void {
		if (this.list.kind === 'ready') {
			this.#setList(this.all.map((p) => (p.id === summary.id ? summary : p)));
		}
		const held = this.#details.get(summary.id);
		if (held?.kind === 'ready') {
			this.#details.set(summary.id, { kind: 'ready', value: { ...held.value, ...summary } });
		}
	}

	/** Changes the list, once it's here: before that, its read brings the change with it. */
	#setList(value: readonly PlaylistSummary[]): void {
		if (this.list.kind !== 'ready') {
			return;
		}
		this.list = {
			kind: 'ready',
			value: value.toSorted((a, b) => a.name.localeCompare(b.name, 'en', { sensitivity: 'base' }))
		};
	}
}
