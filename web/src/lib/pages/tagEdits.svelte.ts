import { MediaQuery, SvelteMap, SvelteSet } from 'svelte/reactivity';
import { library } from '#lib/library/library.svelte.ts';
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
	/** Problems wait for a field to be left: a year half typed isn't wrong yet. */
	readonly #left = new SvelteSet<string>();
	#attempted = $state(false);

	readonly wide = $derived<AlbumWide | null>(this.tags === null ? null : albumWide(this.tags));
	readonly #problems = $derived(
		this.tags === null ? noProblems : problemsIn(this.tags, this.draft)
	);
	readonly summary = $derived<Summary>(
		this.tags === null ? { changes: 0, files: 0, only: null } : summaryOf(this.tags, this.draft)
	);
	/** Whether a problem stands in the way of a write. */
	readonly blocked = $derived(this.#problems.size > 0);
	/** Whether there's anything here to lose. */
	readonly dirty = $derived(this.tags === null ? false : editOf(this.tags, this.draft) !== null);

	constructor(albumId: string, tagger: Tagger, close: () => void) {
		this.albumId = albumId;
		this.#tagger = tagger;
		this.#close = close;
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

	/** Reads the files, carrying what's been touched over to whatever they hold now. */
	async read(): Promise<void> {
		try {
			const fresh = await this.#tagger.albumTags(this.albumId);
			this.draft = rebased(this.draft, fresh);
			this.tags = fresh;
			this.report = null;
		} catch (error) {
			if (error instanceof EditError) {
				this.report = reportOf(error.failure, this.#titleOf(this.tags));
			}
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
		if (edit === null) {
			return { kind: 'idle' };
		}
		this.saving = true;
		this.report = null;
		const { stop, fresh } = await save(this.#tagger, this.albumId, tags.etag, edit);
		this.saving = false;
		if (stop === null) {
			this.#close();
			return { kind: 'done' };
		}
		this.report = reportOf(stop, this.#titleOf(tags));
		if (fresh !== null) {
			this.draft = rebased(this.draft, fresh);
			this.tags = fresh;
		}
		return { kind: 'stopped' };
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

/** Reports whether the album page has a phone's room: board A only. */
export function onPhone(): boolean {
	return phoneWidth.current;
}
