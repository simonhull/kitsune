import { readFileSync } from 'node:fs';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Selection } from './selection.svelte.ts';
import {
	boxShows,
	clickIntent,
	LongPress,
	longPressMs,
	SongList,
	tapOf,
	type Clicked
} from './songRow.ts';

/** An album on two discs, in page order: what a range runs over, across the heading. */
const order = ['1-1', '1-2', '1-3', '2-1', '2-2', '2-3'];

function listOf(selection = new Selection()): { list: SongList; revealed: string[] } {
	const revealed: string[] = [];
	const list = new SongList(selection, {
		order: () => order,
		reveal: (id) => {
			revealed.push(id);
		}
	});
	return { list, revealed };
}

function key(key: string, extra: Partial<KeyboardEventInit> = {}): KeyboardEvent {
	// Node has no KeyboardEvent: as much of one as the list reads.
	let prevented = false;
	return {
		key,
		shiftKey: false,
		ctrlKey: false,
		metaKey: false,
		altKey: false,
		target: null,
		...extra,
		get defaultPrevented() {
			return prevented;
		},
		preventDefault() {
			prevented = true;
		}
	} as unknown as KeyboardEvent;
}

const plain = { shiftKey: false, ctrlKey: false, metaKey: false };

describe('tapOf', () => {
	it('plays on a plain click, picks with Ctrl or Cmd, and takes a range with Shift', () => {
		expect(tapOf(plain, false, false)).toBe('play');
		expect(tapOf({ ...plain, ctrlKey: true }, false, false)).toBe('toggle');
		expect(tapOf({ ...plain, metaKey: true }, false, true)).toBe('toggle');
		expect(tapOf({ ...plain, shiftKey: true }, false, false)).toBe('range');
	});

	it('picks on a tap once a finger is picking, and plays with a mouse all the same', () => {
		expect(tapOf(plain, true, true)).toBe('toggle');
		expect(tapOf(plain, true, false)).toBe('play');
		expect(tapOf(plain, false, true)).toBe('play');
	});
});

describe('LongPress', () => {
	beforeEach(() => {
		vi.useFakeTimers();
	});
	afterEach(() => {
		vi.useRealTimers();
	});

	it('picks after a finger holds, and swallows the click that ends it, once', () => {
		const press = new LongPress();
		let held = 0;
		press.start('touch', () => (held += 1));
		vi.advanceTimersByTime(longPressMs);
		expect(held).toBe(1);
		expect(press.swallow()).toBe(true);
		expect(press.swallow()).toBe(false);
	});

	it('does nothing for a mouse, or a finger that lifts in time', () => {
		const press = new LongPress();
		let held = 0;
		press.start('mouse', () => (held += 1));
		vi.advanceTimersByTime(longPressMs);
		press.start('touch', () => (held += 1));
		vi.advanceTimersByTime(longPressMs - 1);
		press.end();
		vi.advanceTimersByTime(longPressMs);
		expect(held).toBe(0);
		expect(press.swallow()).toBe(false);
	});
});

describe('SongList', () => {
	it('takes a range across a disc heading, in page order', () => {
		const { list } = listOf();
		list.pick('1-2', 'toggle');
		list.pick('2-2', 'range');
		expect(list.selection.inOrder(order)).toEqual(['1-2', '1-3', '2-1', '2-2']);
	});

	it('toggles with Space, and takes the songs passed with Shift and an arrow', () => {
		const { list, revealed } = listOf();
		list.key(key(' '), '1-3');
		expect(list.selection.has('1-3')).toBe(true);
		list.key(key('ArrowDown', { shiftKey: true }), '1-3');
		expect(list.selection.inOrder(order)).toEqual(['1-3', '2-1']);
		expect(revealed).toEqual(['2-1']);
		list.key(key('ArrowUp'), '1-1');
		expect(revealed).toEqual(['2-1']);
	});

	it('takes everything with Ctrl or Cmd+A, and lets go with Esc', () => {
		const { list } = listOf();
		const all = key('a', { ctrlKey: true });
		list.windowKey(all, false);
		expect(all.defaultPrevented).toBe(true);
		expect(list.selection.size).toBe(order.length);
		list.windowKey(key('Escape'), false);
		expect(list.selection.any).toBe(false);
		list.windowKey(key('A', { metaKey: true }), false);
		expect(list.selection.size).toBe(order.length);
	});

	it('leaves the keys alone while a menu or an editor has them', () => {
		const { list } = listOf();
		list.pick('1-1', 'toggle');
		list.windowKey(key('Escape'), true);
		list.windowKey(key('a', { ctrlKey: true }), true);
		expect([...list.selection.ids]).toEqual(['1-1']);
	});

	it('takes a disc with its heading box, and lets it go when it is all picked', () => {
		const selection = new Selection();
		const disc = ['2-1', '2-2', '2-3'];
		selection.toggle('2-2');
		expect(selection.boxFor(disc)).toBe('some');
		selection.toggleAll(disc);
		expect(selection.boxFor(disc)).toBe('all');
		expect(selection.boxFor(order)).toBe('some');
		selection.toggleAll(disc);
		expect(selection.boxFor(disc)).toBe('none');
	});
});

describe('boxShows', () => {
	const rest = { gutterHover: false, focused: false, picked: false, selecting: false };

	it('is hidden at rest, and when only the rest of the row is hovered', () => {
		expect(boxShows(rest)).toBe(false);
	});

	it('shows in the gutter, on focus, on a picked row, and on every row while picking', () => {
		expect(boxShows({ ...rest, gutterHover: true })).toBe(true);
		expect(boxShows({ ...rest, focused: true })).toBe(true);
		expect(boxShows({ ...rest, picked: true, selecting: true })).toBe(true);
		expect(boxShows({ ...rest, selecting: true })).toBe(true);
	});
});

describe('clickIntent', () => {
	/** An element inside a row, given what each selector finds from it. */
	function at(found: Record<string, string | true>): Clicked {
		return {
			closest(selector) {
				const hit = found[selector];
				return hit === undefined
					? null
					: { getAttribute: () => (typeof hit === 'string' ? hit : null) };
			}
		};
	}
	const own = '[data-own-click], a, input, label, button:not(.play)';

	it('plays from anywhere on the row: the title, the number, a cell', () => {
		expect(clickIntent(at({}))).toEqual({ kind: 'play' });
	});

	it('leaves the box, ⋯ and the stars their own clicks', () => {
		expect(clickIntent(at({ '.box input': true, [own]: true }))).toEqual({ kind: 'box' });
		expect(clickIntent(at({ '.more': true, [own]: true }))).toEqual({ kind: 'more' });
		expect(clickIntent(at({ '.stars-input': true, [own]: true }))).toEqual({ kind: 'rate' });
	});

	it('leaves a name that links, the heart, the handle and the gutter to themselves', () => {
		expect(clickIntent(at({ [own]: true }))).toEqual({ kind: 'own' });
	});
});

describe('SongRow', () => {
	it('knows nothing of why a song came up: only Up next fills the second line', () => {
		for (const file of ['SongRow.svelte', 'songRow.ts', 'PickBox.svelte']) {
			const source = readFileSync(new URL(`./${file}`, import.meta.url), 'utf8');
			expect(source).not.toMatch(/shuffle\/why|Why\.svelte/u);
		}
	});

	it('holds no effect or store of its own: props in, events out', () => {
		const source = readFileSync(new URL('./SongRow.svelte', import.meta.url), 'utf8');
		expect(source).not.toMatch(/\$effect|\$state|svelte\/store|library\.svelte/u);
	});
});
