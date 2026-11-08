import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import {
	bulkEditOf,
	bulkProblems,
	bulkSummaryOf,
	bulkTracks,
	emptyBulkDraft,
	etagsOf,
	once,
	findAndReplace,
	numberFromOne,
	titlesFromFiles,
	titlesInTitleCase,
	withBulkField,
	type BulkDraft,
	type BulkKey,
	type BulkTrack
} from '#lib/library/bulk.ts';
import { library } from '#lib/library/library.svelte.ts';
import { EditError, type AlbumTags, type EditFailure } from '#lib/library/source.ts';
import { reportOf, type Report, type Summary } from '#lib/library/tags.ts';

/** Where bulk editing was opened: an album's page (board D), or a selection (board F). */
export type BulkLayout = 'album' | 'selection';

/** What came of asking to write. */
export type BulkWritten =
	/** Every file holds the edit: the first song is on `albumId` now. */
	| { kind: 'done'; albumId: string | undefined }
	/** Something in the draft won't do: this field first. */
	| { kind: 'problem'; field: string }
	/** It stopped, and the report says how far it got. */
	| { kind: 'stopped' }
	| { kind: 'idle' };

const noProblems: ReadonlyMap<string, string> = new Map();

/** One bulk edit: the songs, the albums they're on as read, and what's been touched. */
export class BulkSession {
	readonly layout: BulkLayout;
	/** The songs it's about, in the order they were picked or listed. */
	readonly #ids: string[];
	/** Where each song is, as far as is known. */
	readonly #albumOf: Map<string, string>;

	/** Each album's tags, in the order the songs first name them. Null until read. */
	albums = $state.raw<{ id: string; tags: AlbumTags }[] | null>(null);
	draft = $state.raw<BulkDraft>(emptyBulkDraft());
	saving = $state(false);
	report = $state.raw<Report | null>(null);
	/** The songs this edit will write. */
	readonly picked = new SvelteSet<string>();
	readonly #left = new SvelteSet<string>();
	#attempted = $state(false);

	/** Every song shown: on an album's page that's all of it, picked or not. */
	readonly tracks = $derived<BulkTrack[]>(
		this.albums === null
			? []
			: bulkTracks(this.albums).filter(
					({ id }) => this.layout === 'album' || this.#ids.includes(id)
				)
	);
	readonly pickedTracks = $derived(this.tracks.filter(({ id }) => this.picked.has(id)));
	readonly #problems = $derived(
		this.albums === null ? noProblems : bulkProblems(this.pickedTracks, this.draft)
	);
	readonly summary = $derived<Summary>({
		...bulkSummaryOf(this.pickedTracks, this.draft),
		only: null
	});
	readonly blocked = $derived(this.#problems.size > 0);
	/** For the bar: there's something to write once the files are read. */
	readonly tags = $derived(this.albums);
	readonly dirty = $derived(
		this.albums !== null && bulkEditOf(this.pickedTracks, this.draft, this.#etags()) !== null
	);

	constructor(layout: BulkLayout, ids: readonly string[], albumOf: ReadonlyMap<string, string>) {
		this.layout = layout;
		this.#ids = [...ids];
		this.#albumOf = new SvelteMap(
			ids.flatMap((id) => {
				const album = albumOf.get(id);
				return album === undefined ? [] : [[id, album] as [string, string]];
			})
		);
		for (const id of ids) {
			this.picked.add(id);
		}
	}

	#etags(): Map<string, string> {
		return etagsOf(this.albums ?? []);
	}

	/** The albums the songs are on, each once, in the order the songs name them. */
	get albumIds(): string[] {
		return once(this.#ids.map((id) => this.#albumOf.get(id)).filter((id) => id !== undefined));
	}

	problem(field: string): string | undefined {
		return this.#attempted || this.#left.has(field) ? this.#problems.get(field) : undefined;
	}

	leftField(field: string): void {
		this.#left.add(field);
	}

	#titleOf(id: string): string {
		return this.tracks.find((track) => track.id === id)?.tags.fields.title?.at(0) ?? id;
	}

	/** Reads every album's files. The draft is kept: it's by song and by field. */
	async read(): Promise<void> {
		try {
			const ids = this.albumIds;
			const tags = await Promise.all(ids.map((id) => library.albumTags(id)));
			this.albums = ids.map((id, index) => ({ id, tags: tags[index] as AlbumTags }));
			this.report = null;
		} catch (error) {
			if (error instanceof EditError) {
				this.report = reportOf(error.failure, (id) => this.#titleOf(id));
			}
		}
	}

	/** Finds where songs went: a song written may have moved to another album. */
	async #follow(ids: readonly string[]): Promise<void> {
		await Promise.all(
			ids.map(async (id) => {
				try {
					this.#albumOf.set(id, await library.trackAlbum(id));
				} catch {
					// Gone, or out of reach: the next read says which.
				}
			})
		);
	}

	toggle(id: string): void {
		if (!this.picked.delete(id)) {
			this.picked.add(id);
		}
	}

	/** Picks every song shown, or none once they all are. */
	toggleAll(): void {
		const every = this.tracks.every(({ id }) => this.picked.has(id));
		for (const { id } of this.tracks) {
			if (every) {
				this.picked.delete(id);
			} else {
				this.picked.add(id);
			}
		}
	}

	setField(key: BulkKey, values: string[]): void {
		this.draft = withBulkField(this.pickedTracks, this.draft, key, values);
	}

	numberFromOne(): void {
		this.draft = numberFromOne(this.pickedTracks, this.draft);
	}

	titlesFromFiles(): void {
		this.draft = titlesFromFiles(this.pickedTracks, this.draft);
	}

	titleCase(): void {
		this.draft = titlesInTitleCase(this.pickedTracks, this.draft);
	}

	findAndReplace(find: string, replace: string): void {
		this.draft = findAndReplace(this.pickedTracks, this.draft, find, replace);
	}

	/** Writes the draft to the picked songs' files, in one request. */
	async write(): Promise<BulkWritten> {
		if (this.albums === null || this.saving || this.report?.retry === false) {
			return { kind: 'idle' };
		}
		this.#attempted = true;
		const [field] = this.#problems.keys();
		if (field !== undefined) {
			return { kind: 'problem', field };
		}
		const edit = bulkEditOf(this.pickedTracks, this.draft, this.#etags());
		if (edit === null) {
			return { kind: 'idle' };
		}
		this.saving = true;
		this.report = null;
		try {
			const edited = await library.editTracks(edit);
			return { kind: 'done', albumId: edited.tracks.at(0)?.albumId };
		} catch (error) {
			const failure: EditFailure =
				error instanceof EditError ? error.failure : { kind: 'unavailable' };
			const report = reportOf(failure, (id) => this.#titleOf(id));
			if ('changed' in failure) {
				await this.#follow(failure.changed);
			}
			if (failure.kind === 'stale' || ('changed' in failure && failure.changed.length > 0)) {
				await this.read();
			}
			this.report = report;
			return { kind: 'stopped' };
		} finally {
			this.saving = false;
		}
	}
}

/** The one bulk edit under way: it outlives a page left with it unsaved, for the session. */
class BulkEdits {
	current = $state.raw<BulkSession | null>(null);

	/** Starts editing these songs, dropping any other bulk edit. */
	open(
		layout: BulkLayout,
		ids: readonly string[],
		albumOf: ReadonlyMap<string, string>
	): BulkSession {
		const session = new BulkSession(layout, ids, albumOf);
		this.current = session;
		void session.read();
		return session;
	}

	close(): void {
		this.current = null;
	}

	/** The next user must not inherit a draft. */
	reset(): void {
		this.current = null;
	}
}

export const bulkEdits = new BulkEdits();
