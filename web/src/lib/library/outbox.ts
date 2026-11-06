import { ServerError, type LibrarySource, type PlayReport } from './source.ts';

/** What the server records as this client. */
const clientName = 'web';
/** The most plays kept for one user. Older ones go first. */
const mostKept = 1000;
/** Answers that mean the play can never be taken, so it isn't worth keeping. */
const refused = new Set([400, 404]);

function prefixOf(userId: number): string {
	return `kitsune:play:${String(userId)}:`;
}

/** `crypto.randomUUID` needs a secure context, and this may well be plain HTTP on a home network. */
function newClientId(): string {
	const bytes = crypto.getRandomValues(new Uint8Array(16));
	return Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('');
}

/** What the outbox needs of the server. */
type PlayServer = Pick<LibrarySource, 'sessionUser' | 'reportPlay'>;

/**
 * Plays on their way to the server. Each has a key of its own, because tabs appending to a shared
 * one lose each other's writes, and a play stays until the server has answered for it. Every tab
 * may send the same play: the client ID makes that harmless.
 */
export class Outbox {
	readonly #store: () => Storage;
	readonly #server: PlayServer;
	readonly #flushing = new Set<number>();

	constructor(store: () => Storage, server: PlayServer) {
		this.#store = store;
		this.#server = server;
	}

	/** Keeps a play. Storage that's full or forbidden loses it, and nothing else. */
	add(userId: number, trackId: string, startedAt: number): void {
		const playedAt = new Date(startedAt).toISOString();
		const clientId = newClientId();
		const report: PlayReport = { trackId, playedAt, source: clientName, clientId };
		try {
			const store = this.#store();
			for (const key of this.#waiting(userId).slice(0, 1 - mostKept)) {
				store.removeItem(key);
			}
			store.setItem(`${prefixOf(userId)}${playedAt}:${clientId}`, JSON.stringify(report));
		} catch {
			// A play is worth less than whatever is using the space.
		}
	}

	/** Sends a user's plays, oldest first, until they're gone or the server can't take one. */
	async flush(userId: number): Promise<void> {
		if (this.#flushing.has(userId)) {
			return;
		}
		this.#flushing.add(userId);
		try {
			// Plays added on the way are picked up by going round again.
			for (let keys = this.#waiting(userId); keys.length > 0; keys = this.#waiting(userId)) {
				for (const key of keys) {
					if (!(await this.#deliver(userId, key))) {
						return;
					}
				}
			}
		} catch {
			// Storage went away: there's nothing to send, and the music plays on.
		} finally {
			this.#flushing.delete(userId);
		}
	}

	/** The keys of a user's plays, oldest first: their names sort by time. */
	#waiting(userId: number): string[] {
		const store = this.#store();
		const prefix = prefixOf(userId);
		return Array.from({ length: store.length }, (_, index) => store.key(index))
			.filter((key) => key?.startsWith(prefix) === true)
			.map(String)
			.sort();
	}

	/** Sends one play. Reports whether it's worth trying the next. */
	async #deliver(userId: number, key: string): Promise<boolean> {
		const play = this.#store().getItem(key);
		if (play === null) {
			return true; // Another tab got there first.
		}
		if (!(await this.#stillTheirs(userId))) {
			return false;
		}
		try {
			await this.#server.reportPlay(JSON.parse(play) as PlayReport);
		} catch (error) {
			if (!(error instanceof ServerError && refused.has(error.status))) {
				return false;
			}
		}
		this.#store().removeItem(key);
		return true;
	}

	/**
	 * Checks whose session this is now. Tabs share one cookie: another may have signed someone else
	 * in, and a play sent on their cookie would be theirs.
	 */
	async #stillTheirs(userId: number): Promise<boolean> {
		try {
			return (await this.#server.sessionUser()).id === userId;
		} catch {
			return false;
		}
	}
}
