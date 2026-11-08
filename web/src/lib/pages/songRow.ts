/**
 * What every song row does, whatever list it's in: the logic beside `SongRow.svelte`. A row holds
 * no state of its own. What lasts is the list's (its selection, its order), and the one long press
 * a finger can make at a time, which lives here once for every row.
 */
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

/** Gives a row's play target the focus, in the list under `root`. */
export function focusRow(root: ParentNode | null | undefined, trackId: string): void {
	root?.querySelector<HTMLElement>(`[data-track="${CSS.escape(trackId)}"] .play`)?.focus();
}
