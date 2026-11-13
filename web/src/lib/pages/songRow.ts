/**
 * What every song row does, whatever list it's in: the logic beside `SongRow.svelte`. A row holds
 * no state of its own. What lasts is the list's (its selection, its order), and the one long press
 * a finger can make at a time, which lives here once for every row.
 */
import { keyed, pressed } from '#lib/library/stars.ts';
import { dropPreview, halfUnder, showPreview, valueOf } from '#lib/shell/starsPreview.ts';
import type { Selection } from './selection.svelte.ts';

/** How long a finger holds a row to start picking songs. */
export const longPressMs = 450;

/** What a press on a row's title does. */
export type Tap = 'play' | 'toggle' | 'range';

/** How a row asks its list to pick it: the box, Ctrl or Cmd, or Shift for a range. */
export type PickHow = 'toggle' | 'range';

/**
 * What a click on a row's title does: Shift takes a range, Ctrl or Cmd adds the song, and on a
 * phone, once anything is picked, a tap picks too, so playing waits for the bar. Otherwise it plays.
 */
export function tapOf(
	keys: Pick<MouseEvent, 'shiftKey' | 'ctrlKey' | 'metaKey'>,
	touch: boolean,
	picking: boolean
): Tap {
	if (keys.shiftKey) {
		return 'range';
	}
	return keys.ctrlKey || keys.metaKey || (touch && picking) ? 'toggle' : 'play';
}

/**
 * The long press under way: it picks the song if the finger stays, and the tap that ends it doesn't
 * play. One finger holds one row, so one of these serves every row in the app.
 */
export class LongPress {
	#timer: ReturnType<typeof setTimeout> | undefined;
	#held = false;
	/** Whether a row was last touched by a finger: then, while picking, a tap picks too. */
	touch = false;

	/** A pointer went down on a row. A finger that stays `longPressMs` calls `held`. */
	start(pointerType: string, held: () => void): void {
		this.touch = pointerType === 'touch';
		clearTimeout(this.#timer);
		this.#held = false;
		if (!this.touch) {
			return;
		}
		this.#timer = setTimeout(() => {
			this.#held = true;
			held();
		}, longPressMs);
	}

	/** The finger lifted, slid off or was taken by a scroll. */
	end(): void {
		clearTimeout(this.#timer);
	}

	/** Whether the click now arriving ends a long press, which has done its work already. */
	swallow(): boolean {
		const held = this.#held;
		this.#held = false;
		return held;
	}
}

/** The one long press. */
export const longPress = new LongPress();

/** What bears on whether a row's box shows. */
export interface BoxView {
	/** The pointer is over the gutter, where the boxes are. A finger never hovers. */
	gutterHover: boolean;
	focused: boolean;
	picked: boolean;
	/** Anything in the list is picked. */
	selecting: boolean;
}

/**
 * Whether a row's box shows: while the pointer is in the gutter, on a row with the focus, on a
 * picked row, and on every row while the list is picking. Hovering the rest of the row never shows
 * it, and on touch nothing does until a long press starts picking. SongRow's CSS says this, with
 * `:hover` on the gutter and `data-selecting` on the list, so no row holds any of it.
 */
export function boxShows({ gutterHover, focused, picked, selecting }: BoxView): boolean {
	return gutterHover || focused || picked || selecting;
}

/** A list's songs, as the keyboard and the boxes need them. */
export interface ListOrder {
	/** Every song the list shows, in its order: what a range and select-all run over. */
	order(): readonly string[];
	/** Puts a song's row on screen, if it isn't, and gives it the focus. */
	reveal(trackId: string): void;
}

/** Whether a key went to something that takes text, where Ctrl+A is the field's own. */
function typing(target: EventTarget | null): boolean {
	return (
		typeof HTMLElement !== 'undefined' &&
		target instanceof HTMLElement &&
		(target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName))
	);
}

/**
 * A song list's rules for picking, the same in every list: a box or Ctrl toggles a song, Shift takes
 * a range in the list's order, Space toggles the focused row, Shift with an arrow takes the songs
 * passed, Ctrl or Cmd+A takes everything the list shows, and Esc lets go. A page gives it its
 * selection and its order; nothing here is per row.
 */
export class SongList {
	readonly selection: Selection;
	readonly #list: ListOrder;

	constructor(selection: Selection, list: ListOrder) {
		this.selection = selection;
		this.#list = list;
	}

	pick(trackId: string, how: PickHow): void {
		if (how === 'range') {
			this.selection.extend(trackId, this.#list.order());
		} else {
			this.selection.toggle(trackId);
		}
	}

	/** Space picks the focused song; the arrows move, and with Shift take the songs passed. */
	key(event: KeyboardEvent, trackId: string): void {
		if (event.key === ' ' && !event.ctrlKey && !event.metaKey && !event.altKey) {
			event.preventDefault();
			this.selection.toggle(trackId);
			return;
		}
		if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') {
			return;
		}
		event.preventDefault();
		const order = this.#list.order();
		const at = order.indexOf(trackId);
		const to = at + (event.key === 'ArrowDown' ? 1 : -1);
		const next = to < 0 ? undefined : order.at(to);
		if (at < 0 || next === undefined) {
			return;
		}
		if (event.shiftKey) {
			this.selection.add(trackId);
			this.selection.add(next);
		}
		this.#list.reveal(next);
	}

	/** The header's box: everything the list shows, or nothing when that's all picked already. */
	toggleAll(): void {
		this.selection.toggleAll(this.#list.order());
	}

	/**
	 * A key anywhere on the page: Esc lets the selection go, Ctrl or Cmd+A takes every song. `busy`
	 * says a menu or an editor has the keys, and then neither is the list's.
	 */
	windowKey(event: KeyboardEvent, busy: boolean): void {
		if (busy || event.defaultPrevented) {
			return;
		}
		if (event.key === 'Escape' && this.selection.any) {
			this.selection.clear();
			return;
		}
		if (
			event.key.toLowerCase() === 'a' &&
			(event.ctrlKey || event.metaKey) &&
			!event.altKey &&
			!typing(event.target)
		) {
			event.preventDefault();
			for (const id of this.#list.order()) {
				this.selection.add(id);
			}
		}
	}
}

/** A song's rating slider in a row: StarsInput's. */
const ratingSlider = '.stars-input';

/** What a click inside a row is for. */
export type ClickIntent =
	| { kind: 'play' }
	| { kind: 'box' }
	| { kind: 'more' }
	/** The stars: the rating slider, which the pointer's x reads. */
	| { kind: 'rate' }
	/** Something with its own click: a link, the handle, a column divider. */
	| { kind: 'own' };

/** As much of an element as {@link clickIntent} reads. */
export interface Clicked {
	closest(selector: string): { getAttribute(name: string): string | null } | null;
}

/**
 * What a click on `target`, inside a row, does. Anywhere on a row plays it, except what has a click
 * of its own: the box, ⋯, the stars, and anything else that takes a press (a link's text,
 * the drag handle), which keeps it.
 */
export function clickIntent(target: Clicked): ClickIntent {
	if (target.closest('.box input') !== null) {
		return { kind: 'box' };
	}
	if (target.closest('.more') !== null) {
		return { kind: 'more' };
	}
	if (target.closest(ratingSlider) !== null) {
		return { kind: 'rate' };
	}
	if (target.closest('[data-own-click], a, input, label, button:not(.play)') !== null) {
		return { kind: 'own' };
	}
	return { kind: 'play' };
}

/** What a list does with its rows' events: the row says only which song, and how. */
export interface RowHandlers {
	/** Plays a song: the row says which album it's in, so a list needn't look it up. */
	play(trackId: string, albumId: string): void;
	pick(trackId: string, how: PickHow): void;
	/** Opens the ⋯ menu for a song, at a place. */
	menu(trackId: string, x: number, y: number): void;
	/** Space, the arrows and Shift with them, on a row. */
	key(event: KeyboardEvent, trackId: string): void;
	/** Sets a song's rating, null clearing it: the row has already worked out what a press does. */
	rate(trackId: string, stars: number | null): void;
	picked(trackId: string): boolean;
}

/** The song of the row an event happened in, and the element it happened on. */
function rowEvent(
	event: Event
): { trackId: string; target: Element; row: HTMLElement } | undefined {
	const target = event.target;
	if (!(target instanceof Element)) {
		return undefined;
	}
	const row = target.closest<HTMLElement>('[data-track]');
	const trackId = row?.dataset['track'];
	return row === null || trackId === undefined ? undefined : { trackId, target, row };
}

/**
 * One set of handlers for a whole list of rows, put on its container: a row holds no handler of
 * its own, so the Library's virtual list mounts nothing for them. A plain click plays, Shift takes a
 * range and Ctrl or Cmd adds the song, a right-click opens the menu, a held finger starts picking,
 * and the keys go to the list.
 */
export function rowEvents(handlers: RowHandlers): {
	onclick: (event: MouseEvent) => void;
	oncontextmenu: (event: MouseEvent) => void;
	onpointerdown: (event: PointerEvent) => void;
	onpointermove: (event: PointerEvent) => void;
	onpointerleave: () => void;
	onpointerup: () => void;
	onpointercancel: () => void;
	onkeydown: (event: KeyboardEvent) => void;
} {
	/** The rating showing a preview: one at a time, written to the DOM only (starsPreview.ts). */
	let previewed: HTMLElement | null = null;
	return {
		onclick(event) {
			const at = rowEvent(event);
			if (at === undefined) {
				// A heading's box picks songs too.
				if (event.target instanceof Element && event.target.closest('.pick') !== null) {
					dropText(event.currentTarget);
				}
				return;
			}
			const intent = clickIntent(at.target);
			switch (intent.kind) {
				case 'box': {
					handlers.pick(at.trackId, event.shiftKey ? 'range' : 'toggle');
					dropText(event.currentTarget);
					// The box says what the list did with the click, whatever the click did to it.
					if (at.target instanceof HTMLInputElement) {
						at.target.checked = handlers.picked(at.trackId);
					}
					return;
				}
				case 'more': {
					const button = at.target.closest('.more');
					const { right, bottom } = (button ?? at.target).getBoundingClientRect();
					handlers.menu(at.trackId, right - 300, bottom + 4);
					return;
				}
				case 'rate': {
					const slider = at.target.closest(ratingSlider);
					if (slider !== null) {
						handlers.rate(at.trackId, pressed(valueOf(slider), halfUnder(slider, event.clientX)));
					}
					return;
				}
				case 'own':
					return;
				case 'play': {
					if (longPress.swallow()) {
						event.preventDefault();
						return;
					}
					const picking = at.row.closest('[data-selecting]') !== null;
					const how = tapOf(event, longPress.touch, picking);
					if (how === 'play') {
						handlers.play(at.trackId, at.row.dataset['album'] ?? '');
					} else {
						handlers.pick(at.trackId, how);
						dropText(event.currentTarget);
					}
				}
			}
		},
		oncontextmenu(event) {
			const at = rowEvent(event);
			if (at !== undefined && at.target.closest('a') === null) {
				event.preventDefault();
				handlers.menu(at.trackId, event.clientX, event.clientY);
			}
		},
		onpointerdown(event) {
			const at = rowEvent(event);
			if (at === undefined || at.target.closest('[data-own-click]') !== null) {
				return;
			}
			longPress.start(event.pointerType, () => {
				navigator.vibrate?.(10);
				handlers.pick(at.trackId, 'toggle');
			});
		},
		// A hover over a row's stars shows what a click would set, to the half.
		onpointermove(event) {
			if (event.pointerType === 'touch' || !(event.target instanceof Element)) {
				return;
			}
			const slider = event.target.closest<HTMLElement>(ratingSlider);
			if (slider !== previewed) {
				dropPreview(previewed);
				previewed = slider;
			}
			if (slider !== null) {
				showPreview(slider, halfUnder(slider, event.clientX));
			}
		},
		onpointerleave() {
			dropPreview(previewed);
			previewed = null;
		},
		onpointerup() {
			longPress.end();
		},
		onpointercancel() {
			longPress.end();
		},
		onkeydown(event) {
			const at = rowEvent(event);
			const slider = at?.target.closest(ratingSlider);
			if (at !== undefined && slider != null) {
				const next = keyed(valueOf(slider), event.key);
				if (next !== undefined && !event.altKey && !event.ctrlKey && !event.metaKey) {
					event.preventDefault();
					handlers.rate(at.trackId, next);
				}
				return;
			}
			if (at?.target.closest('.play') != null) {
				handlers.key(event, at.trackId);
			}
		}
	};
}

/**
 * Lets go of any text selected in `list`: picking songs is never selecting their words, and text
 * still highlighted across the rows would read as part of the pick.
 */
function dropText(list: EventTarget | null): void {
	const text = getSelection();
	if (list instanceof Node && text !== null && !text.isCollapsed && text.containsNode(list, true)) {
		text.removeAllRanges();
	}
}

/** Gives a row's play target the focus, in the list under `root`. */
export function focusRow(root: ParentNode | null | undefined, trackId: string): void {
	root?.querySelector<HTMLElement>(`[data-track="${CSS.escape(trackId)}"] .play`)?.focus();
}
