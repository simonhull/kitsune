import { MediaQuery, SvelteMap, SvelteSet } from 'svelte/reactivity';
import { library } from '#lib/library/library.svelte.ts';
import type { RuleChange } from '#lib/library/shuffleRules.ts';
import { EditError, type AlbumTags, type TagField } from '#lib/library/source.ts';
import {
	albumWide,
	editOf,
	emptyDraft,
	problemsIn,
	rebased,
	reportOf,
	save,
	summaryOf,
	withField,
	withTag,
	type AlbumWide,
	type Draft,
	type Report,
	type Summary,
	type Tagger
} from '#lib/library/tags.ts';

/** The album page's own breakpoint. */
const phoneWidth = new MediaQuery('(max-width: 820px)', false);

const noProblems: ReadonlyMap<string, string> = new Map();

/** What came of asking to write. */
export type Written =
	/** Every file holds the edit. */
	| { kind: 'done' }
	/** Something in the draft won't do: this field first. */
	| { kind: 'problem'; field: string }
	/** It stopped, and the report says how far it got. */
	| { kind: 'stopped' }
	/** There was nothing to send, or it can't be sent. */
	| { kind: 'idle' };

/** One album being edited: the files as read, and what's been touched. */
export class TagSession {
	readonly albumId: string;
	readonly #tagger: Tagger;
	readonly #close: () => void;

	/** Null until the files have been read. */
	tags = $state.raw<AlbumTags | null>(null);
	draft = $state.raw<Draft>(emptyDraft());
	saving = $state(false);
	report = $state.raw<Report | null>(null);
	/** The track opened to every tag. */
	#expanded = $state<string | null>(null);
	/** Problems wait for a field to be left: a year half typed isn't wrong yet. */
	readonly #left = new SvelteSet<string>();
	#attempted = $state(false);
	/** The songs the person leaves out of shuffle, as read, and what they've switched. */
	excluded = $state.raw<readonly string[]>([]);
	excludeDraft = $state.raw<Readonly<Record<string, boolean>>>({});

	/** What saving sends to the person's shuffle rules, which are never in a file. */
	readonly ruleChanges = $derived.by<RuleChange[]>(() => {
		const switched = Object.entries(this.excludeDraft);
		return [true, false].flatMap((on) => {
			const tracks = switched.filter(([, want]) => want === on).map(([id]) => id);
			return tracks.length === 0 ? [] : [{ kind: 'exclude' as const, tracks, on }];
		});
	});

	readonly wide = $derived<AlbumWide | null>(this.tags === null ? null : albumWide(this.tags));
	readonly #problems = $derived(
		this.tags === null ? noProblems : problemsIn(this.tags, this.draft)
	);
	readonly summary = $derived.by<Summary>(() => {
		const files =
			this.tags === null ? { changes: 0, files: 0, only: null } : summaryOf(this.tags, this.draft);
		return { ...files, changes: files.changes + Object.keys(this.excludeDraft).length };
	});
	/** Whether a problem stands in the way of a write. */
	readonly blocked = $derived(this.#problems.size > 0);
	/** Whether there's anything here to lose. */
	readonly dirty = $derived(
		Object.keys(this.excludeDraft).length > 0 ||
			(this.tags !== null && editOf(this.tags, this.draft) !== null)
	);

	constructor(albumId: string, tagger: Tagger, close: () => void) {
		this.albumId = albumId;
		this.#tagger = tagger;
		this.#close = close;
	}

	/** The track open to every tag. A phone has only the album page: that board isn't approved. */
	get open(): string | null {
		return phoneWidth.current ? null : this.#expanded;
	}

	expand(id: string | null): void {
		this.#expanded = id;
	}

	/** The problem to show on a field, once it's been left. */
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

	#titleOf(tags: AlbumTags | null): (id: string) => string {
		return (id) => tags?.tracks.find((track) => track.id === id)?.fields.title?.at(0) ?? id;
	}

	/** Whether shuffle leaves a song out, as switched. */
	isExcluded(id: string): boolean {
		return this.excludeDraft[id] ?? this.excluded.includes(id);
	}

	/** Leaves a song out of the person's shuffle, or lets it back: saved with the tags. */
	setExcluded(id: string, on: boolean): void {
		const others = Object.fromEntries(
			Object.entries(this.excludeDraft).filter(([key]) => key !== id)
		);
		this.excludeDraft = on === this.excluded.includes(id) ? others : { ...others, [id]: on };
	}

	/** Reads the files, carrying what's been touched over to whatever they hold now. */
	async read(): Promise<void> {
		try {
			const fresh = await this.#tagger.albumTags(this.albumId);
			this.draft = rebased(this.draft, fresh);
			this.tags = fresh;
			this.report = null;
			void this.#readRules(fresh.tracks.map(({ id }) => id));
		} catch (error) {
			if (error instanceof EditError) {
				this.report = reportOf(error.failure, this.#titleOf(this.tags));
			}
		}
	}

	async #readRules(ids: readonly string[]): Promise<void> {
		try {
			this.excluded = (await library.shuffleRules(ids)).excluded;
		} catch {
			// Not known: the switches read as off, and saving one still says what it wants.
		}
	}

	setYear(year: string): void {
		this.draft = { ...this.draft, year };
	}

	setGenres(genres: string[]): void {
		this.draft = { ...this.draft, genres };
	}

	setField(id: string, field: TagField, values: string[]): void {
		if (this.tags !== null) {
			this.draft = withField(this.tags, this.draft, id, field, values);
		}
	}

	setTag(id: string, key: string, values: string[]): void {
		if (this.tags !== null) {
			this.draft = withTag(this.tags, this.draft, id, key, values);
		}
	}

	/** Writes the draft to the files, in one request. */
	async write(): Promise<Written> {
		const { tags, draft } = this;
		if (tags === null || this.saving || this.report?.retry === false) {
			return { kind: 'idle' };
		}
		this.#attempted = true;
		const [field] = this.#problems.keys();
		if (field !== undefined) {
			return { kind: 'problem', field };
		}
		const edit = editOf(tags, draft);
		const rules = this.ruleChanges;
		if (edit === null && rules.length === 0) {
			return { kind: 'idle' };
		}
		this.saving = true;
		this.report = null;
		const { stop, fresh } =
			edit === null
				? { stop: null, fresh: null }
				: await save(this.#tagger, this.albumId, tags.etag, edit);
		if (stop === null) {
			const saved = await this.#saveRules(rules);
			this.saving = false;
			if (saved) {
				this.#close();
				return { kind: 'done' };
			}
			return { kind: 'stopped' };
		}
		this.saving = false;
		this.report = reportOf(stop, this.#titleOf(tags));
		if (fresh !== null) {
			this.draft = rebased(this.draft, fresh);
			this.tags = fresh;
		}
		return { kind: 'stopped' };
	}

	/** Saves the shuffle switches, after the files. It reports false, and says why, when it can't. */
	async #saveRules(rules: readonly RuleChange[]): Promise<boolean> {
		try {
			await library.changeShuffleRules(rules);
			return true;
		} catch {
			this.report = {
				message: "Your shuffle rules couldn't be saved. Try again.",
				detail: undefined,
				written: [],
				unwritten: [],
				retry: true
			};
			return false;
		}
	}
}

/**
 * Every album being edited. It outlives the album page, so unsaved edits are still there on a
 * return, for as long as the session lasts.
 */
export class TagEdits {
	readonly #tagger: Tagger;
	readonly #sessions = new SvelteMap<string, TagSession>();

	constructor(tagger: Tagger) {
		this.#tagger = tagger;
	}

	of(albumId: string): TagSession | undefined {
		return this.#sessions.get(albumId);
	}

	/** Opens an album for editing. */
	open(albumId: string): void {
		if (!this.#sessions.has(albumId)) {
			const session = new TagSession(albumId, this.#tagger, () => {
				this.close(albumId);
			});
			this.#sessions.set(albumId, session);
			void session.read();
		}
	}

	/** Drops an album's edits, whatever they hold. */
	close(albumId: string): void {
		this.#sessions.delete(albumId);
	}

	/** Takes someone back to edits left behind: the files may have moved on since. */
	arrive(albumId: string): void {
		void this.#sessions.get(albumId)?.read();
	}

	/** Lets go of an album left with nothing to lose. */
	depart(albumId: string): void {
		if (this.#sessions.get(albumId)?.dirty === false) {
			this.close(albumId);
		}
	}

	/** The next user must not inherit a draft. */
	reset(): void {
		this.#sessions.clear();
	}
}

export const tagEdits = new TagEdits(library);
