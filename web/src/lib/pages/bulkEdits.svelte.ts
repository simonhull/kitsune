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
import {
	emptyRulesDraft,
	moveIn,
	noRules,
	orderOf,
	ruleChanges,
	type RuleChange,
	type RulesDraft
} from '#lib/library/shuffleRules.ts';
import {
	EditError,
	type AlbumTags,
	type EditFailure,
	type ShuffleRules
} from '#lib/library/source.ts';
import { reportOf, type Report, type Summary } from '#lib/library/tags.ts';

/** What a bulk edit knows of a song before reading its file: enough to list it. */
export interface SongInfo {
	id: string;
	albumId: string;
	title: string;
	trackNo?: number | undefined;
}

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
	readonly #info: Map<string, SongInfo>;
	/** Tags can be read and written: off, only the Shuffle tab works. */
	readonly canTag: boolean;

	/** Each album's tags, in the order the songs first name them. Null until read. */
	albums = $state.raw<{ id: string; tags: AlbumTags }[] | null>(null);
	draft = $state.raw<BulkDraft>(emptyBulkDraft());
	saving = $state(false);
	report = $state.raw<Report | null>(null);
	/** The songs this edit will write. */
	readonly picked = new SvelteSet<string>();
	readonly #left = new SvelteSet<string>();
	#attempted = $state(false);
	/** The person's shuffle rules touching the songs, as read. */
	rules = $state.raw<ShuffleRules>(noRules);
	rulesRead = $state(false);
	rulesDraft = $state.raw<RulesDraft>(emptyRulesDraft());

	/** Every song shown: on an album's page that's all of it, picked or not. */
	readonly tracks = $derived.by<BulkTrack[]>(() => {
		if (this.albums !== null) {
			return bulkTracks(this.albums).filter(
				({ id }) => this.layout === 'album' || this.#ids.includes(id)
			);
		}
		return this.canTag
			? []
			: this.#ids.flatMap((id) => {
					const info = this.#info.get(id);
					return info === undefined ? [] : [unreadTrack(info)];
				});
	});
	readonly pickedTracks = $derived(this.tracks.filter(({ id }) => this.picked.has(id)));
	readonly pickedIds = $derived(this.pickedTracks.map(({ id }) => id));
	readonly #problems = $derived(
		this.albums === null ? noProblems : bulkProblems(this.pickedTracks, this.draft)
	);
	/** What saving the Shuffle tab sends. */
	readonly ruleChanges = $derived<RuleChange[]>(
		ruleChanges(this.pickedIds, this.rules, this.rulesDraft)
	);
	readonly summary = $derived.by<Summary>(() => {
		const files =
			this.albums === null
				? { changes: 0, files: 0 }
				: bulkSummaryOf(this.pickedTracks, this.draft);
		return { changes: files.changes + this.ruleChanges.length, files: files.files, only: null };
	});
	readonly blocked = $derived(this.#problems.size > 0);
	/** For the bar: there's something to save once what it's about is read. */
	readonly tags = $derived.by(() =>
		this.canTag ? this.albums : this.rulesRead ? this.tracks : null
	);
	readonly dirty = $derived(
		this.ruleChanges.length > 0 ||
			(this.albums !== null && bulkEditOf(this.pickedTracks, this.draft, this.#etags()) !== null)
	);

	constructor(layout: BulkLayout, songs: readonly SongInfo[], canTag: boolean) {
		this.layout = layout;
		this.canTag = canTag;
		this.#ids = songs.map(({ id }) => id);
		this.#info = new SvelteMap(songs.map((song) => [song.id, song]));
		this.#albumOf = new SvelteMap(songs.map(({ id, albumId }) => [id, albumId]));
		for (const { id } of songs) {
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

	/** Reads every album's files and the person's rules. The drafts are kept: they're by song. */
	async read(): Promise<void> {
		await Promise.all([this.#readRules(), this.canTag ? this.#readTags() : Promise.resolve()]);
	}

	async #readRules(): Promise<void> {
		try {
			this.rules = await library.shuffleRules(this.#ids);
			this.rulesRead = true;
		} catch {
			this.report = rulesTrouble("Couldn't read your shuffle rules.");
		}
	}

	setCombine(on: boolean): void {
		this.rulesDraft = { ...this.rulesDraft, combine: on };
	}

	setExclude(on: boolean): void {
		this.rulesDraft = { ...this.rulesDraft, exclude: on };
	}

	/** Moves a song of the run, as a drag or Alt with an arrow does. */
	moveInRun(from: number, to: number): void {
		const order = orderOf(this.pickedIds, this.rules, this.rulesDraft);
		this.rulesDraft = { ...this.rulesDraft, order: moveIn(order, from, to) };
	}

	async #readTags(): Promise<void> {
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

	/**
	 * Writes the draft to the picked songs' files, in one request, then saves the shuffle rules,
	 * which are never in a file: one Save does both, and rules save even where files can't.
	 */
	async write(): Promise<BulkWritten> {
		if (this.saving || this.report?.retry === false) {
			return { kind: 'idle' };
		}
		this.#attempted = true;
		const [field] = this.#problems.keys();
		if (field !== undefined) {
			return { kind: 'problem', field };
		}
		const edit =
			this.albums === null ? null : bulkEditOf(this.pickedTracks, this.draft, this.#etags());
		const changes = this.ruleChanges;
		if (edit === null && changes.length === 0) {
			return { kind: 'idle' };
		}
		this.saving = true;
		this.report = null;
		try {
			const edited = edit === null ? null : await library.editTracks(edit);
			try {
				await library.changeShuffleRules(changes);
			} catch {
				await this.#readRules();
				this.rulesDraft = emptyRulesDraft();
				this.report = rulesTrouble(
					edit === null
						? "Your shuffle rules couldn't be saved. Try again."
						: "The tags were written, but your shuffle rules couldn't be saved. Try again."
				);
				return { kind: 'stopped' };
			}
			return { kind: 'done', albumId: edited?.tracks.at(0)?.albumId };
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

/** A song as listed before its file is read: its title and number from the library. */
function unreadTrack(info: SongInfo): BulkTrack {
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

/** The one bulk edit under way: it outlives a page left with it unsaved, for the session. */
class BulkEdits {
	current = $state.raw<BulkSession | null>(null);

	/** Starts editing these songs, dropping any other bulk edit. */
	open(layout: BulkLayout, songs: readonly SongInfo[], canTag: boolean): BulkSession {
		const session = new BulkSession(layout, songs, canTag);
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
