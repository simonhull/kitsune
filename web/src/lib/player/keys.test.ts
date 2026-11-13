import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { playerKey, seekStepSeconds, togglesOnSpace } from './keys';

/** An element that is, or sits inside, what `is` names: `button`, `input`, `[role="slider"]`. */
class FakeElement {
	readonly #is: string | null;

	constructor(is: string | null = null) {
		this.#is = is;
	}

	closest(selector: string): object | null {
		return this.#is !== null && selector.split(', ').includes(this.#is) ? this : null;
	}
}

function press(extra: Partial<KeyboardEvent> = {}): KeyboardEvent {
	return {
		code: 'Space',
		key: ' ',
		target: new FakeElement(),
		defaultPrevented: false,
		...extra
	} as unknown as KeyboardEvent;
}

function key(name: string, extra: Partial<KeyboardEvent> = {}): KeyboardEvent {
	return press({ code: `Key${name.toUpperCase()}`, key: name, ...extra });
}

const on = (is: string) => ({ target: new FakeElement(is) as never });

beforeEach(() => {
	vi.stubGlobal('Element', FakeElement);
});

afterEach(() => {
	vi.unstubAllGlobals();
});

describe('togglesOnSpace', () => {
	it('takes Space with nothing focused', () => {
		expect(togglesOnSpace(press())).toBe(true);
	});

	it('leaves Space to a button, a link or a field', () => {
		expect(togglesOnSpace(press(on('button')))).toBe(false);
		expect(togglesOnSpace(press(on('a[href]')))).toBe(false);
		expect(togglesOnSpace(press(on('textarea')))).toBe(false);
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

describe('playerKey', () => {
	it('plays or pauses on Space, as before', () => {
		expect(playerKey(press())).toEqual({ kind: 'toggle' });
		expect(playerKey(press(on('button')))).toBeNull();
	});

	it('seeks 10 s on ← and →, held down too', () => {
		expect(seekStepSeconds).toBe(10);
		expect(playerKey(key('ArrowLeft', { code: 'ArrowLeft' }))).toEqual({ kind: 'seek', by: -10 });
		expect(playerKey(key('ArrowRight', { code: 'ArrowRight' }))).toEqual({ kind: 'seek', by: 10 });
		expect(playerKey(key('ArrowRight', { code: 'ArrowRight', repeat: true }))).toEqual({
			kind: 'seek',
			by: 10
		});
	});

	it('goes to the song before or after on Shift+← and Shift+→, once per press', () => {
		expect(playerKey(key('ArrowLeft', { shiftKey: true }))).toEqual({ kind: 'previous' });
		expect(playerKey(key('ArrowRight', { shiftKey: true }))).toEqual({ kind: 'next' });
		expect(playerKey(key('ArrowRight', { shiftKey: true, repeat: true }))).toBeNull();
	});

	it('leaves ← and → to whatever focused uses them: a field, a slider, a toolbar, a divider', () => {
		for (const is of [
			'input',
			'[role="slider"]',
			'[role="toolbar"]',
			'[role="separator"]',
			'[role="listbox"]',
			'[role="menu"]'
		]) {
			expect(playerKey(key('ArrowLeft', on(is)))).toBeNull();
		}
		// A button, a song's row or a link doesn't use them: the player does.
		expect(playerKey(key('ArrowLeft', on('button')))).toEqual({ kind: 'seek', by: -10 });
	});

	it('shuffles on S and moves Repeat on with R', () => {
		expect(playerKey(key('s'))).toEqual({ kind: 'shuffle' });
		expect(playerKey(key('r'))).toEqual({ kind: 'repeat' });
		expect(playerKey(key('S', { shiftKey: true }))).toBeNull();
	});

	it('rates the song playing on 1 to 5, and clears it on 0', () => {
		for (const n of [1, 2, 3, 4, 5]) {
			expect(playerKey(key(String(n)))).toEqual({ kind: 'rate', stars: n });
		}
		expect(playerKey(key('0'))).toEqual({ kind: 'rate', stars: null });
		expect(playerKey(key('6'))).toBeNull();
	});

	it('opens Now playing on N', () => {
		expect(playerKey(key('n'))).toEqual({ kind: 'now-playing' });
	});

	it('takes nothing in a text field, with a modifier other than Shift, held, or taken already', () => {
		for (const name of ['s', 'r', 'n', '3', '0']) {
			expect(playerKey(key(name, on('input')))).toBeNull();
			expect(playerKey(key(name, on('textarea')))).toBeNull();
			expect(playerKey(key(name, on('[contenteditable]')))).toBeNull();
			expect(playerKey(key(name, { repeat: true }))).toBeNull();
			expect(playerKey(key(name, { ctrlKey: true }))).toBeNull();
			expect(playerKey(key(name, { metaKey: true }))).toBeNull();
			expect(playerKey(key(name, { altKey: true }))).toBeNull();
		}
		expect(playerKey(key('ArrowLeft', { ctrlKey: true }))).toBeNull();
		expect(playerKey(key('s', { defaultPrevented: true }))).toBeNull();
		expect(playerKey(key('Enter'))).toBeNull();
	});
});
