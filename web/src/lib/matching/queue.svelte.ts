import { matchSheet } from './sheet.svelte.ts';
import type { MatchCandidate, QueuedAlbum } from './types.ts';

/** How far ahead the queue asks: the album on show and this many after it. */
export const askAhead = 2;

/** What the queue knows of an album's releases. */
export type Asked =
	| { kind: 'waiting' }
	| { kind: 'asking' }
	| { kind: 'ready'; candidates: MatchCandidate[] }
	| { kind: 'failed' };

/**
 * The queue's place and what it has asked, kept while the page is away: Esc stops for now, and the
 * place is there to come back to.
 */
class QueuePlace {
	/** The album on show. */
	current = $state<string | null>(null);
	/** The last album matched here, for the done strip. */
	done = $state.raw<{ id: string; title: string; files: number } | null>(null);
	asked = $state.raw<Record<string, Asked>>({});

	/** Asks for the album on show, and works ahead for the next ones. */
	askFor(todo: readonly QueuedAlbum[]): void {
		const at = Math.max(
			0,
			todo.findIndex(({ id }) => id === this.current)
		);
		todo.slice(at, at + 1 + askAhead).forEach(({ id }, index) => {
			if (this.asked[id] !== undefined && this.asked[id].kind !== 'failed') {
				return;
			}
			this.asked = { ...this.asked, [id]: { kind: 'asking' } };
			const found = index === 0 ? matchSheet.matches(id) : this.#ahead(id);
			found
				.then(({ candidates }) => {
					this.asked = { ...this.asked, [id]: { kind: 'ready', candidates } };
				})
				.catch(() => {
					this.asked = { ...this.asked, [id]: { kind: 'failed' } };
				});
		});
	}

	/** Asks ahead, behind whatever a person waits on, and keeps it where the sheet looks. */
	#ahead(id: string): Promise<{ candidates: MatchCandidate[] }> {
		matchSheet.prefetch(id);
		return matchSheet.matches(id);
	}

	/** Forgets what was asked of an album, as after it changed. */
	forget(id: string): void {
		this.asked = Object.fromEntries(Object.entries(this.asked).filter(([key]) => key !== id));
	}
}

export const queuePlace = new QueuePlace();
