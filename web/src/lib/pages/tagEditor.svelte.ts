/**
 * The one tag editor's session: the songs, from one album or several, which of them are ticked, a
 * draft by song with the shared fields over the ticked ones, and the person's shuffle rules for them.
 * It writes through `POST /albums/{id}/edits` when every song that changes is on one album, and
 * `POST /edits` otherwise. One editor is open at a time (`tagEditor`), drawn by the shell over
 * whatever page opened it, and its edits go with that page (`editorHistory`).
 */
import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import {
	editTracks,
	emptyEditDraft,
	etagsOf,
	findAndReplace,
	numberFromOne,
	once,
	problemsOf,
	rebased,
	requestOf,
	summaryOf,
	titlesFromFiles,
	titlesInTitleCase,
	withField,
	withOwn,
	withShared,
	withTag,
	type EditDraft,
	type EditTrack,
	type SharedKey
} from '#lib/library/edit.ts';
import { library } from '#lib/library/library.svelte.ts';
import {
	emptyRulesDraft,
	excludeChanges,
	moveIn,
	noRules,
	orderOf,
	runChanges,
	type RuleChange,
	type RulesDraft
} from '#lib/library/shuffleRules.ts';
import {
	EditError,
	type AlbumEdit,
	type AlbumTags,
	type EditedTracks,
	type EditFailure,
	type FieldValues,
	type ShuffleRules,
	type TagField,
	type TracksEdit
} from '#lib/library/source.ts';
import { reportOf, save, type Report, type Summary, type Tagger } from '#lib/library/tags.ts';

/** What the editor knows of a song before reading its file: enough to list it. */
export interface SongInfo {
	id: string;
	albumId: string;
	title: string;
	trackNo?: number | undefined;
}

/** What to open the editor on. */
export type Opening =
	/** An album, every song listed: those named ticked (all of them when left out), one open. */
	| { kind: 'album'; albumId: string; ticked?: readonly string[]; open?: string | undefined }
	/** Some songs. All on one album, they open as that album with only them ticked. */
	| { kind: 'songs'; songs: readonly SongInfo[] };

/** How the editor is headed and listed: one album (board A), or a selection (board C). */
export type EditorShape = 'album' | 'selection';

/** What the editor needs of the library, which keeps everything on screen up to date. */
export interface Editing extends Tagger {
	editTracks(edit: TracksEdit): Promise<EditedTracks>;
	trackAlbum(trackId: string): Promise<string>;
	shuffleRules(trackIds: readonly string[]): Promise<ShuffleRules>;
	changeShuffleRules(changes: readonly RuleChange[]): Promise<void>;
	movedTo(id: string): string | undefined;
}

/** What came of asking to write. */
export type Written =
	/**
	 * Every file holds the edit, and the rules are saved. `albumId` is where the first song written
	 * is now; `matched`, the album a match filled the fields of.
	 */
	| { kind: 'done'; albumId: string | undefined; matched: string | null }
	/** Something in the draft won't do: this field first. */
	| { kind: 'problem'; field: string }
	/** It stopped, and the report says how far it got. */
	| { kind: 'stopped' }
	/** There was nothing to send, or it can't be sent. */
	| { kind: 'idle' };

const noProblems: ReadonlyMap<string, string> = new Map();

/** A song as listed before its file is read: its title and number from the library. */
function unread(info: SongInfo): EditTrack {
	const fields = {
		title: [info.title],
		...(info.trackNo === undefined ? {} : { track: [String(info.trackNo)] })
	};
	return {
		id: info.id,
		albumId: info.albumId,
		tags: { id: info.id, file: info.title, fields, tags: [] }
	};
}

/** A report of rules that couldn't be read or saved: trying again may work. */
function rulesTrouble(message: string): Report {
	return { message, detail: undefined, written: [], unwritten: [], retry: true };
}

export class EditSession {
	/** Names this editor in history: a new one is a new entry. */
	readonly key: string;
	readonly shape: EditorShape;
	/** Tags can be read and written: off, only the Shuffle tab works. */
	readonly canTag: boolean;
	readonly #editing: Editing;
	/** A selection's songs, in the order picked; null for an album, which lists all of its own. */
	readonly #ids: readonly string[] | null;
	readonly #info: ReadonlyMap<string, SongInfo>;
	/** Where each song is, as far as is known. */
	readonly #albumOf: SvelteMap<string, string>;
	/** What to tick once an album's songs are known. */
	readonly #ticking: readonly string[] | null;
	/** An album's own editor starts from its ID, before its songs are known. */
	readonly #seed: string | undefined;

	/** Each album's tags, in the order the songs first name them. Null until read. */
	albums = $state.raw<{ id: string; tags: AlbumTags }[] | null>(null);
	draft = $state.raw<EditDraft>(emptyEditDraft());
	/** The songs the shared fields and the Shuffle tab act on. */
	readonly ticked = new SvelteSet<string>();
	/** The song open to every tag, if any. */
	open = $state<string | null>(null);
	saving = $state(false);
	report = $state.raw<Report | null>(null);
	/** The album a match filled the fields of, recorded as matched once they're written. */
	matched = $state<string | null>(null);
	/** Problems wait for a field to be left: a year half typed isn't wrong yet. */
	readonly #left = new SvelteSet<string>();
	#attempted = $state(false);
	/** The person's shuffle rules touching the songs, as read, and what's been switched. */
	rules = $state.raw<ShuffleRules>(noRules);
	rulesRead = $state(false);
	rulesDraft = $state.raw<RulesDraft>(emptyRulesDraft());
	/** One song's own Exclude switch, from its every-tag view. */
	excludeDraft = $state.raw<Readonly<Record<string, boolean>>>({});

	/** Every song listed: an album's whole, or a selection's own. */
	readonly tracks = $derived.by<EditTrack[]>(() => {
		if (this.albums !== null) {
			const ids = this.#ids;
			const all = editTracks(this.albums);
			return ids === null ? all : all.filter(({ id }) => ids.includes(id));
		}
		return this.canTag
			? []
			: (this.#ids ?? []).flatMap((id) => {
					const info = this.#info.get(id);
					return info === undefined ? [] : [unread(info)];
				});
	});
	readonly tickedTracks = $derived(this.tracks.filter(({ id }) => this.ticked.has(id)));
	readonly tickedIds = $derived(this.tickedTracks.map(({ id }) => id));
	/** The album every ticked song is on, when there's one: Find matches works on it. */
	readonly oneAlbum = $derived.by<string | null>(() => {
		const albums = once(this.tickedTracks.map(({ albumId }) => albumId));
		return albums.length === 1 ? (albums[0] ?? null) : null;
	});
	readonly #problems = $derived(
		this.albums === null ? noProblems : problemsOf(this.tracks, this.draft, this.ticked)
	);
	/** What saving sends to the person's shuffle rules, which are never in a file. */
	readonly ruleChanges = $derived.by<RuleChange[]>(() => [
		...runChanges(this.tickedIds, this.rules, this.rulesDraft),
		...excludeChanges(
			this.tracks.map(({ id }) => id),
			this.rules,
			(id) => this.isExcluded(id)
		)
	]);
	readonly summary = $derived.by<Summary>(() => {
		const files =
			this.albums === null
				? { changes: 0, files: 0, only: null }
				: summaryOf(this.tracks, this.draft, this.ticked);
		return { ...files, changes: files.changes + this.ruleChanges.length };
	});
	/** Whether a problem stands in the way of a write. */
	readonly blocked = $derived(this.#problems.size > 0);
	/** Whether what it's about has been read: the dock has something to count. */
	readonly ready = $derived.by(() => (this.canTag ? this.albums !== null : this.rulesRead));

	constructor(key: string, opening: Opening, canTag: boolean, editing: Editing) {
		this.key = key;
		this.canTag = canTag;
		this.#editing = editing;
		if (opening.kind === 'album') {
			this.shape = 'album';
			this.#ids = null;
			this.#info = new SvelteMap();
			this.#albumOf = new SvelteMap();
			this.#ticking = opening.ticked ?? null;
			this.#seed = opening.albumId;
			this.open = opening.open ?? null;
		} else {
			this.shape = 'selection';
			this.#ids = opening.songs.map(({ id }) => id);
			this.#info = new SvelteMap(opening.songs.map((song) => [song.id, song]));
			this.#albumOf = new SvelteMap(opening.songs.map(({ id, albumId }) => [id, albumId]));
			this.#ticking = null;
			this.#seed = undefined;
			for (const { id } of opening.songs) {
				this.ticked.add(id);
			}
		}
	}

	/** The albums the songs are on, each once, in the order the songs name them. */
	get albumIds(): string[] {
		if (this.#ids === null) {
			return this.#seed === undefined ? [] : [this.#seed];
		}
		return once(this.#ids.map((id) => this.#albumOf.get(id)).filter((id) => id !== undefined));
	}

	/** How many songs it's about: a selection's from the start, an album's once read. */
	get songCount(): number {
		return this.#ids?.length ?? this.tracks.length;
	}

	/** The problem to show on a field, once it's been left or a write was tried. */
	problem(field: string): string | undefined {
		return this.#attempted || this.#left.has(field) ? this.#problems.get(field) : undefined;
	}

	/** Reports whether a field's change is held back by a problem, shown or not. */
	hasProblem(field: string): boolean {
		return this.#problems.has(field);
	}

	leftField(field: string): void {
		this.#left.add(field);
	}

	track(id: string): EditTrack | undefined {
		return this.tracks.find((track) => track.id === id);
	}

	#titleOf(id: string): string {
		const track = this.track(id);
		return track?.tags.fields.title?.at(0) ?? track?.tags.file ?? this.#info.get(id)?.title ?? id;
	}

	/** Reads every album's files, then the person's rules for what they hold. Drafts are kept. */
	async read(): Promise<void> {
		if (this.canTag) {
			await this.#readTags();
		}
		await this.#readRules();
	}

	async #readTags(): Promise<void> {
		try {
			const ids = this.albumIds;
			const tags = await Promise.all(ids.map((id) => this.#editing.albumTags(id)));
			const first = this.albums === null;
			this.albums = ids.map((id, index) => ({ id, tags: tags[index] as AlbumTags }));
			this.report = null;
			if (first && this.#ids === null) {
				const ticking = this.#ticking;
				for (const { id } of this.tracks) {
					if (ticking === null || ticking.includes(id)) {
						this.ticked.add(id);
					}
				}
			}
		} catch (error) {
			if (error instanceof EditError) {
				this.report = reportOf(error.failure, (id) => this.#titleOf(id));
			}
		}
	}

	async #readRules(): Promise<void> {
		const ids = this.#ids ?? this.tracks.map(({ id }) => id);
		if (ids.length === 0) {
			return;
		}
		try {
			this.rules = await this.#editing.shuffleRules(ids);
			this.rulesRead = true;
		} catch {
			this.report ??= rulesTrouble("Couldn't read your shuffle rules.");
		}
	}

	toggle(id: string): void {
		if (!this.ticked.delete(id)) {
			this.ticked.add(id);
		}
	}

	/** Ticks every song listed, or none once they all are. */
	toggleAll(): void {
		const every = this.tracks.every(({ id }) => this.ticked.has(id));
		for (const { id } of this.tracks) {
			if (every) {
				this.ticked.delete(id);
			} else {
				this.ticked.add(id);
			}
		}
	}

	/** Opens a song to every tag, or goes back to them all. */
	expand(id: string | null): void {
		this.open = id;
	}

	setShared(key: SharedKey, values: string[]): void {
		this.draft = withShared(this.tickedTracks, this.draft, key, values);
	}

	setField(id: string, field: TagField, values: string[]): void {
		const track = this.track(id);
		if (track !== undefined) {
			this.draft = withField(track, this.draft, this.ticked, field, values);
		}
	}

	setTag(id: string, key: string, values: string[]): void {
		const track = this.track(id);
		if (track !== undefined) {
			this.draft = withTag(track, this.draft, key, values);
		}
	}

	numberFromOne(): void {
		this.draft = numberFromOne(this.tickedTracks, this.draft);
	}

	titlesFromFiles(): void {
		this.draft = titlesFromFiles(this.tickedTracks, this.draft);
	}

	titleCase(): void {
		this.draft = titlesInTitleCase(this.tickedTracks, this.draft);
	}

	findAndReplace(find: string, replace: string): void {
		this.draft = findAndReplace(this.tickedTracks, this.draft, find, replace);
	}

	/** Takes what a match proposes into the fields, like any change: nothing is written yet. */
	fill(albumId: string, edit: AlbumEdit): void {
		const values = new SvelteMap<string, FieldValues>(
			(edit.tracks ?? []).map(({ id, set }) => [id, set ?? {}])
		);
		this.draft = withOwn(this.tracks, this.draft, values);
		this.matched = albumId;
	}

	/** Whether shuffle leaves a song out, as switched. */
	isExcluded(id: string): boolean {
		const own = this.excludeDraft[id];
		if (own !== undefined) {
			return own;
		}
		const all = this.rulesDraft.exclude;
		return all !== null && this.ticked.has(id) ? all : this.rules.excluded.includes(id);
	}

	/** One song's own switch, from its every-tag view. */
	setExcluded(id: string, on: boolean): void {
		const others = Object.fromEntries(
			Object.entries(this.excludeDraft).filter(([key]) => key !== id)
		);
		this.excludeDraft = { ...others, [id]: on };
	}

	/** The Shuffle tab's switch: every ticked song, over any one switched on its own. */
	setExclude(on: boolean): void {
		this.excludeDraft = Object.fromEntries(
			Object.entries(this.excludeDraft).filter(([id]) => !this.ticked.has(id))
		);
		this.rulesDraft = { ...this.rulesDraft, exclude: on };
	}

	/** Whether the Shuffle tab's Exclude reads on: every ticked song is left out. */
	get allExcluded(): boolean {
		return this.tickedIds.length > 0 && this.tickedIds.every((id) => this.isExcluded(id));
	}

	setCombine(on: boolean): void {
		this.rulesDraft = { ...this.rulesDraft, combine: on };
	}

	/** Moves a song of the run, as a drag or Alt with an arrow does. */
	moveInRun(from: number, to: number): void {
		const order = orderOf(this.tickedIds, this.rules, this.rulesDraft);
		this.rulesDraft = { ...this.rulesDraft, order: moveIn(order, from, to) };
	}

	/**
	 * Writes the draft to the files in one request, then saves the shuffle rules, which are never in
	 * a file: one Write does both, and rules save even where files can't.
	 */
	async write(): Promise<Written> {
		if (this.saving || this.report?.retry === false) {
			return { kind: 'idle' };
		}
		this.#attempted = true;
		const [field] = this.#problems.keys();
		if (field !== undefined) {
			return { kind: 'problem', field };
		}
		const albums = this.albums;
		const request =
			albums === null ? null : requestOf(this.tracks, this.draft, this.ticked, etagsOf(albums));
		const rules = this.ruleChanges;
		if (request === null && rules.length === 0) {
			return { kind: 'idle' };
		}
		this.saving = true;
		this.report = null;
		try {
			let landed: string | undefined;
			if (request?.route === 'album') {
				const { stop, fresh } = await save(
					this.#editing,
					request.albumId,
					request.etag,
					request.edit
				);
				if (stop !== null) {
					this.report = reportOf(stop, (id) => this.#titleOf(id));
					if (fresh !== null) {
						this.#adopt(request.albumId, fresh);
					}
					return { kind: 'stopped' };
				}
				landed = this.#editing.movedTo(request.albumId) ?? request.albumId;
			} else if (request?.route === 'tracks') {
				const edited = await this.#editTracks(request.edit);
				if (edited === null) {
					return { kind: 'stopped' };
				}
				landed = edited.tracks.at(0)?.albumId;
			}
			if (!(await this.#saveRules(rules, request !== null))) {
				return { kind: 'stopped' };
			}
			return { kind: 'done', albumId: landed, matched: this.matched };
		} finally {
			this.saving = false;
		}
	}

	/** Takes an album's files as they are now, carrying the draft over to them. */
	#adopt(albumId: string, fresh: AlbumTags): void {
		const before = this.tracks.filter((track) => track.albumId === albumId).map(({ id }) => id);
		this.draft = rebased(this.draft, fresh, before);
		this.albums = (this.albums ?? []).map((album) =>
			album.id === albumId ? { id: albumId, tags: fresh } : album
		);
	}

	/** Sends an edit across albums. On a stop it follows the songs and reads them again. */
	async #editTracks(edit: TracksEdit): Promise<EditedTracks | null> {
		try {
			return await this.#editing.editTracks(edit);
		} catch (error) {
			const failure: EditFailure =
				error instanceof EditError ? error.failure : { kind: 'unavailable' };
			const report = reportOf(failure, (id) => this.#titleOf(id));
			if ('changed' in failure) {
				await this.#follow(failure.changed);
			}
			if (failure.kind === 'stale' || ('changed' in failure && failure.changed.length > 0)) {
				await this.#readTags();
			}
			this.report = report;
			return null;
		}
	}

	/** Finds where songs went: a song written may have moved to another album. */
	async #follow(ids: readonly string[]): Promise<void> {
		await Promise.all(
			ids.map(async (id) => {
				try {
					this.#albumOf.set(id, await this.#editing.trackAlbum(id));
				} catch {
					// Gone, or out of reach: the next read says which.
				}
			})
		);
	}

	/** Saves the shuffle switches, after the files. It reports false, and says why, when it can't. */
	async #saveRules(rules: readonly RuleChange[], wrote: boolean): Promise<boolean> {
		if (rules.length === 0) {
			return true;
		}
		try {
			await this.#editing.changeShuffleRules(rules);
			return true;
		} catch {
			await this.#readRules();
			this.rulesDraft = emptyRulesDraft();
			this.excludeDraft = {};
			this.report = rulesTrouble(
				wrote
					? "The tags were written, but your shuffle rules couldn't be saved. Try again."
					: "Your shuffle rules couldn't be saved. Try again."
			);
			return false;
		}
	}
}

/** The one editor open: a new one takes its place, and the next user inherits none. */
export class TagEditor {
	current = $state.raw<EditSession | null>(null);
	readonly #editing: Editing;
	#opened = 0;

	constructor(editing: Editing) {
		this.#editing = editing;
	}

	/**
	 * Opens the editor. `canTag` says whether the files can be changed: without it only the Shuffle
	 * tab works, on the songs as the library lists them.
	 */
	open(opening: Opening, canTag: boolean): EditSession {
		const session = new EditSession(
			`tags:${String(++this.#opened)}`,
			canTag ? asAlbum(opening) : opening,
			canTag,
			this.#editing
		);
		this.current = session;
		void session.read();
		return session;
	}

	close(): void {
		this.current = null;
	}

	reset(): void {
		this.current = null;
	}
}

/** Songs all on one album open as that album, with only them ticked. */
function asAlbum(opening: Opening): Opening {
	if (opening.kind === 'album') {
		return opening;
	}
	const [albumId, ...others] = once(opening.songs.map((song) => song.albumId));
	return albumId === undefined || others.length > 0
		? opening
		: { kind: 'album', albumId, ticked: opening.songs.map(({ id }) => id) };
}

export const tagEditor = new TagEditor(library);
