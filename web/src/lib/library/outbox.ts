import { ServerError, type LibrarySource, type ListenReport, type PlayReport } from './source.ts';

/** What the server records as this client. */
const clientName = 'web';
/** The most reports of one kind kept for one user. Older ones go first. */
const mostKept = 1000;
/** Answers that mean the report can never be taken, so it isn't worth keeping. */
const refused = new Set([400, 404]);

/** `crypto.randomUUID` needs a secure context, and this may well be plain HTTP on a home network. */
export function newClientId(): string {
	const bytes = crypto.getRandomValues(new Uint8Array(16));
	return Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('');
}

/** What a box needs of the server, besides sending. */
type Server = Pick<LibrarySource, 'sessionUser'>;

/**
 * Reports on their way to the server. Each has a key of its own, because tabs appending to a shared
 * one lose each other's writes, and a report stays until the server has answered for it. Every tab
 * may send the same one: its client ID makes that harmless.
 */
class Mailbox<T> {
	readonly #store: () => Storage;
	readonly #server: Server;
	readonly #kind: string;
	readonly #send: (report: T) => Promise<void>;
	readonly #flushing = new Set<number>();

	constructor(
		store: () => Storage,
		server: Server,
		kind: string,
		send: (report: T) => Promise<void>
	) {
		this.#store = store;
		this.#server = server;
		this.#kind = kind;
		this.#send = send;
	}

	#prefixOf(userId: number): string {
		return `kitsune:${this.#kind}:${String(userId)}:`;
	}

	/** Keeps a report under a name that sorts by `at`. Storage that's full or forbidden loses it. */
	post(userId: number, at: string, name: string, report: T): void {
		try {
			const store = this.#store();
			for (const key of this.#waiting(userId).slice(0, 1 - mostKept)) {
				store.removeItem(key);
			}
			store.setItem(`${this.#prefixOf(userId)}${at}:${name}`, JSON.stringify(report));
		} catch {
			// A report is worth less than whatever is using the space.
		}
	}

	/** Sends a user's reports, oldest first, until they're gone or the server can't take one. */
	async flush(userId: number): Promise<void> {
		if (this.#flushing.has(userId)) {
			return;
		}
		this.#flushing.add(userId);
		try {
			// Reports added on the way are picked up by going round again.
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

	/** The keys of a user's reports, oldest first: their names sort by time. */
	#waiting(userId: number): string[] {
		const store = this.#store();
		const prefix = this.#prefixOf(userId);
		return Array.from({ length: store.length }, (_, index) => store.key(index))
			.filter((key) => key?.startsWith(prefix) === true)
			.map(String)
			.sort();
	}

	/** Sends one report. Reports whether it's worth trying the next. */
	async #deliver(userId: number, key: string): Promise<boolean> {
		const report = this.#store().getItem(key);
		if (report === null) {
			return true; // Another tab got there first.
		}
		if (!(await this.#stillTheirs(userId))) {
			return false;
		}
		try {
			await this.#send(JSON.parse(report) as T);
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
	 * in, and a report sent on their cookie would be theirs.
	 */
	async #stillTheirs(userId: number): Promise<boolean> {
		try {
			return (await this.#server.sessionUser()).id === userId;
		} catch {
			return false;
		}
	}
}

/** Plays on their way to the server. */
export class Outbox {
	readonly #mail: Mailbox<PlayReport>;

	constructor(store: () => Storage, server: Pick<LibrarySource, 'sessionUser' | 'reportPlay'>) {
		this.#mail = new Mailbox(store, server, 'play', (play) => server.reportPlay(play));
	}

	/** Keeps a play. Storage that's full or forbidden loses it, and nothing else. */
	add(userId: number, trackId: string, startedAt: number): void {
		const playedAt = new Date(startedAt).toISOString();
		const clientId = newClientId();
		this.#mail.post(userId, playedAt, clientId, {
			trackId,
			playedAt,
			source: clientName,
			clientId
		});
	}

	/** Sends a user's plays, oldest first, until they're gone or the server can't take one. */
	flush(userId: number): Promise<void> {
		return this.#mail.flush(userId);
	}
}

/**
 * Listens on their way to the server: each start, and each end. A listen's start and end share its
 * client ID, so the server makes them one, in whichever order they arrive.
 */
export class ListenOutbox {
	readonly #mail: Mailbox<ListenReport>;

	constructor(store: () => Storage, server: Pick<LibrarySource, 'sessionUser' | 'reportListen'>) {
		this.#mail = new Mailbox(store, server, 'listen', (listen) => server.reportListen(listen));
	}

	/** Keeps a listen's start, or with `ended`, its end. */
	add(userId: number, report: ListenReport): void {
		const phase = report.ended === undefined ? 'a' : 'b';
		this.#mail.post(userId, report.startedAt, `${report.clientId}:${phase}`, report);
	}

	flush(userId: number): Promise<void> {
		return this.#mail.flush(userId);
	}
}
