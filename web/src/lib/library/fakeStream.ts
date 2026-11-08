import type { EventStream } from './live.ts';

/** An event stream a test drives, as a browser's would behave. Only tests use it. */
export class FakeStream implements EventStream {
	readyState = 0;
	onopen: ((event: Event) => void) | null = null;
	onerror: ((event: Event) => void) | null = null;
	closed = false;
	readonly #listeners = new Map<string, ((event: MessageEvent<string>) => void)[]>();

	addEventListener(type: string, listener: (event: MessageEvent<string>) => void): void {
		this.#listeners.set(type, [...(this.#listeners.get(type) ?? []), listener]);
	}

	close(): void {
		this.closed = true;
		this.readyState = 2;
	}

	open(): void {
		this.readyState = 1;
		this.onopen?.(new Event('open'));
	}

	send(type: string, data: unknown): void {
		this.sendRaw(type, JSON.stringify(data));
	}

	sendRaw(type: string, data: string): void {
		const event = new MessageEvent<string>(type, { data });
		for (const listener of this.#listeners.get(type) ?? []) {
			listener(event);
		}
	}

	/** The connection dropped; the browser tries again by itself, or gives up. */
	fail(givesUp: boolean): void {
		this.readyState = givesUp ? 2 : 0;
		this.onerror?.(new Event('error'));
	}
}
