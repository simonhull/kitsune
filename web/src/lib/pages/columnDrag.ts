/**
 * Moving a song column, by a drag of its header label or by Alt and the arrows: the DOM side of
 * columnMove.ts. While a column moves, only its rule in columnMotion.ts changes, and those of the
 * columns making room, and the lane and the hole, two elements of their own: no row and no state. The order commits in the task that ends the move, through `commit`, which
 * must update the DOM before it returns (flushSync); the settle that follows only closes the last
 * few pixels between the pointer and the column's slot.
 */
import { flushSync } from 'svelte';
import type { MovableId } from '#lib/library/source.ts';
import { keyTarget, labelOf, moveSaid, wallSaid } from './columns.ts';
import { moveFrame, movedTo, type MoveFrame, type Slot } from './columnMove.ts';
import { drawAt, moved, rest, wait } from './columnMotion.ts';

/** How far a press travels before the column lifts: a mouse or pen, and a finger. */
export const liftAfter = { mouse: 4, touch: 8 } as const;

/** How long each phase lasts before the list is let go, at full motion. */
const settleMs = 360;
const cancelMs = 220;
const flipMs = 220;
const landedMs = 700;
const bumpMs = 220;
const resetMs = 280;

/** What a move needs of the header it starts in. */
export interface MoveHost {
	/** The element whose custom properties every header and row of the list reads. */
	list: HTMLElement;
	/** The header the label is in. */
	header: HTMLElement;
	/** What the list places, in order: the slots a column moves among. */
	lineup: readonly MovableId[];
	/** Puts `id` at `to` in the lineup, and updates the DOM before returning. */
	commit: (id: MovableId, to: number, keys: boolean) => void;
	/** Holds another device's change while the pointer has a column. */
	hold: (holding: boolean) => void;
}

export function reducedMotion(): boolean {
	return matchMedia('(prefers-reduced-motion: reduce)').matches;
}

let liveRegion: HTMLElement | null = null;

/** Says something politely to a screen reader: one region for every list. */
export function say(text: string): void {
	if (liveRegion === null || !liveRegion.isConnected) {
		liveRegion = document.createElement('div');
		liveRegion.className = 'column-live';
		liveRegion.setAttribute('aria-live', 'polite');
		liveRegion.setAttribute('role', 'status');
		document.body.append(liveRegion);
	}
	liveRegion.textContent = text;
}

/** Each list's timer for letting go of its last move, so a new move can take over from it. */
const releases = new WeakMap<HTMLElement, ReturnType<typeof setTimeout>>();

function cellOf(header: HTMLElement, id: MovableId): HTMLElement | null {
	return header.querySelector<HTMLElement>(`:scope > [data-col="${id}"]`);
}

/** Lets go of a list after a move: its phase, its offsets, its lane and hole. */
function release(list: HTMLElement, after: number): void {
	clearTimeout(releases.get(list));
	releases.set(
		list,
		setTimeout(() => {
			releases.delete(list);
			clearNow(list);
		}, after)
	);
}

function clearNow(list: HTMLElement): void {
	clearTimeout(releases.get(list));
	releases.delete(list);
	delete list.dataset['moving'];
	delete list.dataset['phase'];
	delete list.dataset['held'];
	rest();
	for (const each of list.querySelectorAll('.column-lane, .column-hole')) {
		each.remove();
	}
}

/** Stamps the label of a column that has landed: the thud, the fox ink and the scuff. */
function land(header: HTMLElement, id: MovableId): void {
	const cell = cellOf(header, id);
	if (cell === null) {
		return;
	}
	delete cell.dataset['lifted'];
	delete cell.dataset['landed'];
	void cell.offsetWidth;
	cell.dataset['landed'] = '';
	setTimeout(() => {
		delete cell.dataset['landed'];
	}, landedMs);
}

/** Bumps a label against a wall. */
function bump(cell: HTMLElement, side: 'left' | 'right'): void {
	delete cell.dataset['bump'];
	void cell.offsetWidth;
	cell.dataset['bump'] = side;
	setTimeout(() => {
		delete cell.dataset['bump'];
	}, bumpMs);
}

/** Where each column's header cell starts, by id: what a move measures before and after. */
function lefts(header: HTMLElement, lineup: readonly MovableId[]): Map<MovableId, number> {
	const from = header.getBoundingClientRect().left;
	return new Map(
		lineup.flatMap((id) => {
			const cell = cellOf(header, id);
			return cell === null ? [] : [[id, cell.getBoundingClientRect().left - from] as const];
		})
	);
}

/**
 * Moves columns to where `change` puts them, each animating from where it was: Alt and an arrow,
 * Reset and its Undo. `change` must update the DOM before it returns.
 */
export function flip(
	lists: readonly { list: HTMLElement; header: HTMLElement; lineup: readonly MovableId[] }[],
	change: () => void,
	{ phase = 'flip', stagger = 0 }: { phase?: 'flip' | 'reset'; stagger?: number } = {}
): void {
	const before = lists.map(({ header, lineup }) => lefts(header, lineup));
	for (const { list } of lists) {
		clearNow(list);
		list.dataset['moving'] = '';
		list.dataset['phase'] = 'commit';
	}
	flushSync(change);
	const still = reducedMotion();
	lists.forEach(({ header }, index) => {
		const ids = [...header.querySelectorAll<HTMLElement>(':scope > [data-col]')].flatMap((cell) => {
			const id = cell.dataset['col'] as MovableId | undefined;
			return id === undefined ? [] : [id];
		});
		const after = lefts(header, ids);
		ids.forEach((id, at) => {
			const was = before[index]?.get(id);
			const now = after.get(id);
			if (!still && was !== undefined && now !== undefined && was !== now) {
				drawAt(id, was - now);
			}
			if (stagger > 0) {
				wait(id, at * stagger);
			}
		});
	});
	// One layout, so each column starts its way from where it was.
	void lists[0]?.list.offsetWidth;
	for (const { list } of lists) {
		list.dataset['phase'] = phase;
		for (const id of moved()) {
			drawAt(id, 0);
		}
		const count = list.querySelectorAll(':scope .header > [data-col]').length;
		release(list, phase === 'reset' ? resetMs + count * stagger + 40 : flipMs);
	}
}

/** Every list on the page with a header, for Reset: each moves its own columns home. */
export function listsOnPage(): { list: HTMLElement; header: HTMLElement; lineup: MovableId[] }[] {
	return [...document.querySelectorAll<HTMLElement>('.song-list')].flatMap((list) => {
		const header = list.querySelector<HTMLElement>('.header');
		if (header === null) {
			return [];
		}
		const lineup = [...header.querySelectorAll<HTMLElement>(':scope > [data-col]')].flatMap(
			(cell) => {
				const id = cell.dataset['col'] as MovableId | undefined;
				return id === undefined ? [] : [id];
			}
		);
		return [{ list, header, lineup }];
	});
}

/** Alt with an arrow, Home or End on a label: one place, or to an end, or a bump at the wall. */
export function keyMove(host: MoveHost, id: MovableId, key: string, cell: HTMLElement): void {
	const from = host.lineup.indexOf(id);
	const to = keyTarget(from, host.lineup.length, key);
	if (to === null) {
		if (!reducedMotion()) {
			bump(cell, key === 'ArrowLeft' || key === 'Home' ? 'left' : 'right');
		}
		say(wallSaid(id, key));
		return;
	}
	const lineup = movedTo(host.lineup, from, to);
	flip([host], () => {
		host.commit(id, to, true);
	});
	land(host.header, id);
	cellOf(host.header, id)?.querySelector<HTMLElement>('.label')?.focus({ preventScroll: true });
	say(moveSaid(lineup, id));
}

/**
 * A column under the pointer, from the lift to the drop or the cancel. The slots are read once, at
 * the lift; each move after that writes custom properties, and only those that changed.
 */
export class ColumnDrag {
	readonly #host: MoveHost;
	readonly #id: MovableId;
	readonly #held: number;
	readonly #slots: Slot[];
	readonly #gap: number;
	readonly #lane: HTMLElement;
	readonly #hole: HTMLElement;
	readonly #cell: HTMLElement;
	#frame: MoveFrame;
	#done = false;

	constructor(host: MoveHost, id: MovableId) {
		this.#host = host;
		this.#id = id;
		const { list, header, lineup } = host;
		clearNow(list);
		this.#held = lineup.indexOf(id);
		const origin = header.getBoundingClientRect();
		this.#slots = lineup.map((each) => {
			const rect = cellOf(header, each)?.getBoundingClientRect();
			return rect === undefined
				? { left: 0, width: 0 }
				: { left: rect.left - origin.left, width: rect.width };
		});
		this.#gap = Number.parseFloat(getComputedStyle(header).columnGap) || 12;
		this.#frame = moveFrame(this.#slots, this.#held, 0, this.#gap);
		const cell = cellOf(header, id);
		if (cell === null) {
			throw new Error(`no header cell for ${id}`);
		}
		this.#cell = cell;

		this.#lane = document.createElement('div');
		this.#lane.className = 'column-lane';
		this.#hole = document.createElement('div');
		this.#hole.className = 'column-hole';
		this.#hole.innerHTML =
			'<span class="column-hole-head"></span><span class="column-hole-rows"></span>';
		// Beside the rows, in whatever layer they paint in, so the held cells can stand above the lane.
		let home: HTMLElement = header;
		while (home !== list && home.parentElement !== null && home.querySelector('.row') === null) {
			home = home.parentElement;
		}
		home.append(this.#hole, this.#lane);
		this.#place(origin);

		list.dataset['moving'] = '';
		list.dataset['phase'] = 'drag';
		list.dataset['held'] = id;
		delete cell.dataset['pressing'];
		cell.dataset['lifted'] = '';
		document.documentElement.dataset['grabbing'] = '';
		host.hold(true);
		say(`Picked up ${labelOf(id)}, ${String(this.#held + 1)} of ${String(lineup.length)}.`);
	}

	/**
	 * Puts the lane and the hole over the held column's slot, in the coordinates of whatever they're
	 * placed in, from the header down to the last row on screen.
	 */
	#place(origin: DOMRect): void {
		const { list } = this.#host;
		const slot = this.#slots.at(this.#held) ?? { left: 0, width: 0 };
		const parent = this.#lane.offsetParent ?? list;
		const box = parent.getBoundingClientRect();
		const left = origin.left - box.left + slot.left;
		const headTop = origin.top - box.top;
		const headBottom = origin.bottom - box.top;
		const rows = [...list.querySelectorAll<HTMLElement>('.row[data-track]')];
		const lowest = Math.max(
			origin.bottom,
			...rows.map((row) => row.getBoundingClientRect().bottom).filter((y) => y <= innerHeight)
		);
		const bottom = Math.min(lowest, innerHeight) - box.top;
		Object.assign(this.#lane.style, {
			left: `${String(left - 8)}px`,
			width: `${String(slot.width + 16)}px`,
			top: `${String(headBottom - 4)}px`,
			height: `${String(Math.max(0, bottom - headBottom + 10))}px`
		});
		Object.assign(this.#hole.style, {
			left: `${String(left)}px`,
			width: `${String(slot.width)}px`,
			top: `${String(headTop)}px`,
			height: `${String(Math.max(0, bottom - headTop))}px`
		});
		this.#hole.style.setProperty('--head-height', `${String(origin.height)}px`);
	}

	/** The pointer is `dx` from where it pressed. */
	move(dx: number): void {
		if (this.#done) {
			return;
		}
		const frame = moveFrame(this.#slots, this.#held, dx, this.#gap);
		this.#frame = frame;
		const { lineup } = this.#host;
		frame.shifts.forEach((shift, index) => {
			const id = lineup.at(index);
			if (id !== undefined) {
				drawAt(id, shift);
			}
		});
		this.#lane.style.translate = `${String(frame.offset)}px 0`;
		const own = this.#slots.at(this.#held)?.left ?? 0;
		this.#hole.style.translate = `${String(frame.holeLeft - own)}px 0`;
	}

	/** Lets go: the new order in this task, then the settle and the stamp. */
	drop(): void {
		if (this.#done) {
			return;
		}
		const { target, offset, holeLeft } = this.#frame;
		if (target === this.#held) {
			this.cancel(true);
			return;
		}
		this.#done = true;
		const { list, header, lineup } = this.#host;
		const own = this.#slots.at(this.#held)?.left ?? 0;
		const from = reducedMotion() ? 0 : offset - (holeLeft - own);
		list.dataset['phase'] = 'commit';
		flushSync(() => {
			this.#host.commit(this.#id, target, false);
		});
		for (const id of lineup) {
			drawAt(id, 0);
		}
		drawAt(this.#id, from);
		this.#lane.style.translate = `${String(from)}px 0`;
		this.#lane.style.left = `${String(Number.parseFloat(this.#lane.style.left) + holeLeft - own)}px`;
		this.#hole.remove();
		// One layout with the column at its new slot, offset by what's left to go, then the settle.
		void list.offsetWidth;
		list.dataset['phase'] = 'settle';
		drawAt(this.#id, 0);
		this.#lane.style.translate = '0px 0';
		this.#lane.dataset['landing'] = '';
		land(header, this.#id);
		this.#end();
		say(moveSaid(movedTo(lineup, this.#held, target), this.#id));
		release(list, settleMs);
	}

	/** Esc, a lost pointer, or a drop in place: everything falls home, and nothing is saved. */
	cancel(inPlace = false): void {
		if (this.#done) {
			return;
		}
		this.#done = true;
		const { list, lineup } = this.#host;
		list.dataset['phase'] = 'cancel';
		for (const id of lineup) {
			drawAt(id, 0);
		}
		this.#lane.style.translate = '0px 0';
		this.#lane.dataset['landing'] = '';
		delete this.#cell.dataset['lifted'];
		this.#end();
		const at = `${String(this.#held + 1)} of ${String(lineup.length)}`;
		say(inPlace ? `${labelOf(this.#id)} stays ${at}.` : `${labelOf(this.#id)} put back, ${at}.`);
		release(list, cancelMs);
	}

	#end(): void {
		delete document.documentElement.dataset['grabbing'];
		this.#host.hold(false);
	}
}
