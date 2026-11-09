import { library } from '#lib/library/library.svelte.ts';
import { EditError } from '#lib/library/source.ts';
import { reportOf, type Report } from '#lib/library/tags.ts';
import { findMatches, MatchError, reviewMatch, setMatchState, type MatchFailure } from './api.ts';
import { editOf, initialChoices, summaryOf, type Basis, type Choices } from './review.ts';
import type { MatchCandidate, MatchList, MatchTerms } from './types.ts';

/** Something read from the server: on its way, here, or not to be had. */
export type Load<T> =
	{ kind: 'loading' } | { kind: 'ready'; value: T } | { kind: 'failed'; failure: MatchFailure };

/** Where the sheet was opened from, and what it does once the songs are written. */
export interface Opening {
	albumId: string;
	from: 'album' | 'editor' | 'bulk' | 'queue';
	/** The songs to tick, a selection's. All of them when left out. */
	only?: readonly string[];
	/** After Apply wrote: the album the songs are on now, and how many files took it. */
	applied?: (albumId: string, files: number) => void;
}

/** How a step arrives. Going back is a move of its own, never the last one played in reverse. */
export type Motion = 'rise' | 'grow' | 'back';

/** A review, with what's needed to send its edit. */
export interface Held extends Basis {
	etag: string;
	candidate: MatchCandidate;
}

function failureOf(error: unknown): MatchFailure {
	return error instanceof MatchError
		? error.failure
		: { kind: 'unavailable', retryAfterSeconds: 10 };
}

const termsKey = (albumId: string, terms: MatchTerms): string =>
	JSON.stringify([albumId, terms.artist ?? '', terms.album ?? '', terms.code ?? '']);

/**
 * The one match sheet: over the album page, the tag editor and the queue. It holds the releases
 * found, the release being reviewed and what's ticked, and sends one edit on Apply.
 */
class MatchSheet {
	current = $state.raw<Opening | null>(null);
	step = $state<'list' | 'review'>('list');
	motion = $state<Motion>('rise');
	terms = $state.raw<MatchTerms>({});
	list = $state.raw<Load<MatchList>>({ kind: 'loading' });
	review = $state.raw<Load<Held>>({ kind: 'loading' });
	choices = $state.raw<Choices | null>(null);
	applying = $state(false);
	report = $state.raw<Report | null>(null);
	/** Answers kept for the session, and the queue's asks ahead, by album and terms. */
	readonly #found = new Map<string, Promise<MatchList>>();
	#asked = 0;

	open(opening: Opening): void {
		this.current = opening;
		this.terms = {};
		this.step = 'list';
		this.motion = 'rise';
		this.report = null;
		void this.find({});
	}

	close(): void {
		this.current = null;
		this.choices = null;
		this.review = { kind: 'loading' };
	}

	/** Asks for an album's releases ahead, so opening it later finds them waiting. */
	prefetch(albumId: string): void {
		const key = termsKey(albumId, {});
		if (!this.#found.has(key)) {
			const asked = findMatches(albumId, {}, true);
			asked.catch(() => this.#found.delete(key));
			this.#found.set(key, asked);
		}
	}

	/** Whether an album's releases are already here, or on their way. */
	asked(albumId: string): boolean {
		return this.#found.has(termsKey(albumId, {}));
	}

	/** The releases for an album, from what's kept when it's there. */
	matches(albumId: string, terms: MatchTerms = {}): Promise<MatchList> {
		const key = termsKey(albumId, terms);
		let found = this.#found.get(key);
		if (found === undefined) {
			found = findMatches(albumId, terms);
			found.catch(() => this.#found.delete(key));
			this.#found.set(key, found);
		}
		return found;
	}

	/** Asks again, for other terms or after a failure. */
	async find(terms: MatchTerms): Promise<void> {
		const opening = this.current;
		if (opening === null) {
			return;
		}
		const asked = ++this.#asked;
		this.terms = terms;
		this.list = { kind: 'loading' };
		try {
			const value = await this.matches(opening.albumId, terms);
			if (asked === this.#asked) {
				this.list = { kind: 'ready', value };
			}
		} catch (error) {
			if (asked === this.#asked) {
				this.list = { kind: 'failed', failure: failureOf(error) };
			}
		}
	}

	/** Grows the sheet into the review of one release. */
	async pick(candidate: MatchCandidate): Promise<void> {
		const opening = this.current;
		if (opening === null) {
			return;
		}
		const asked = ++this.#asked;
		this.step = 'review';
		this.motion = 'grow';
		this.report = null;
		this.review = { kind: 'loading' };
		this.choices = null;
		try {
			const [review, tags] = await Promise.all([
				reviewMatch(candidate.id),
				library.albumTags(opening.albumId),
				library.loadAlbum(opening.albumId)
			]);
			const album = library.album(opening.albumId);
			const lengths = Object.fromEntries((album?.tracks ?? []).map((t) => [t.id, t.durationMs]));
			const held: Held = { review, tags: tags.tracks, lengths, etag: tags.etag, candidate };
			if (asked !== this.#asked) {
				return;
			}
			let choices = initialChoices(held);
			if (opening.only !== undefined) {
				const only = new Set(opening.only);
				choices = {
					...choices,
					songs: Object.fromEntries(
						Object.entries(choices.songs).map(([id, on]) => [id, on && only.has(id)])
					)
				};
			}
			this.review = { kind: 'ready', value: held };
			this.choices = choices;
		} catch (error) {
			if (asked === this.#asked) {
				this.review = {
					kind: 'failed',
					failure: error instanceof EditError ? { kind: 'gone' } : failureOf(error)
				};
			}
		}
	}

	/** Back to the releases: the list slides in from the left, as a move of its own. */
	back(): void {
		++this.#asked;
		this.step = 'list';
		this.motion = 'back';
		this.report = null;
	}

	/** Writes what's ticked, through the tag editor's own path, and records the album as matched. */
	async apply(): Promise<void> {
		const opening = this.current;
		const review = this.review;
		if (opening === null || review.kind !== 'ready' || this.choices === null || this.applying) {
			return;
		}
		const edit = editOf(review.value, this.choices);
		const files = summaryOf(review.value, this.choices).files;
		if ((edit.tracks ?? []).length === 0) {
			return;
		}
		this.applying = true;
		this.report = null;
		try {
			await library.editAlbum(opening.albumId, review.value.etag, edit);
			const landed = library.movedTo(opening.albumId) ?? opening.albumId;
			// The queue's word for it: a failure here costs only the Matched tab its row.
			await setMatchState(landed, 'matched').catch(() => undefined);
			this.forget(opening.albumId);
			this.close();
			opening.applied?.(landed, files);
		} catch (error) {
			if (error instanceof EditError) {
				const titles = new Map(
					review.value.tags.map((t) => [t.id, t.fields.title?.at(0) ?? t.file])
				);
				this.report = reportOf(error.failure, (id) => titles.get(id) ?? id);
			} else {
				throw error;
			}
		} finally {
			this.applying = false;
		}
	}

	/** Forgets what was found for an album, which has changed. */
	forget(albumId: string): void {
		for (const key of this.#found.keys()) {
			if ((JSON.parse(key) as string[])[0] === albumId) {
				this.#found.delete(key);
			}
		}
	}

	/** Forgets everything, as at sign-out. */
	reset(): void {
		this.close();
		this.#found.clear();
	}
}

export const matchSheet = new MatchSheet();
