import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { togglesOnSpace } from './keys';

class FakeElement {
	readonly #claimed: boolean;

	constructor(claimed: boolean) {
		this.#claimed = claimed;
	}

	closest(): object | null {
		return this.#claimed ? this : null;
	}
}

function press(extra: Partial<KeyboardEvent> = {}): KeyboardEvent {
	return { code: 'Space', target: new FakeElement(false), ...extra } as unknown as KeyboardEvent;
}

describe('togglesOnSpace', () => {
	beforeEach(() => {
		vi.stubGlobal('Element', FakeElement);
	});

	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it('takes Space with nothing focused', () => {
		expect(togglesOnSpace(press())).toBe(true);
	});

	it('leaves Space to a button, a link or a field', () => {
		expect(togglesOnSpace(press({ target: new FakeElement(true) as never }))).toBe(false);
	});

	it('takes nothing else', () => {
		expect(togglesOnSpace(press({ code: 'Enter' }))).toBe(false);
		expect(togglesOnSpace(press({ shiftKey: true }))).toBe(false);
		expect(togglesOnSpace(press({ metaKey: true }))).toBe(false);
	});

	it('ignores a held key', () => {
		expect(togglesOnSpace(press({ repeat: true }))).toBe(false);
	});
});
