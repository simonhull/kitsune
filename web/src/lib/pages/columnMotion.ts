/**
 * Where each column is drawn while it moves: one style rule per column, in a sheet of its own,
 * `.song-list[data-moving] [data-col='genre'] { translate: … }`. A move writes a rule's `translate`,
 * and only that column's cells restyle. A custom property set on the list did the same, but every
 * element under the list inherits it and restyles at each write: measured on the Library at 1920,
 * 59 ms a write against 3.5 for a rule (plan 052's Outcome).
 */
import type { MovableId } from '#lib/library/source.ts';
import { columnSpecs } from './columns.ts';

/** Every column a list places, each with its rule. */
export const motionIds: readonly MovableId[] = [
	'number',
	'title',
	...columnSpecs.map(({ id }) => id)
];

/** A column's rule, at rest. */
export function motionRule(id: MovableId): string {
	return `.song-list[data-moving] [data-col='${id}'] { translate: 0px 0; transition-delay: 0ms; }`;
}

let rules: Map<MovableId, CSSStyleRule> | null = null;
/** What each rule says now, so a write that changes nothing is skipped. */
const offsets = new Map<MovableId, number>();
const waits = new Map<MovableId, number>();

function sheet(): Map<MovableId, CSSStyleRule> {
	if (rules !== null) {
		return rules;
	}
	const made = new CSSStyleSheet();
	for (const id of motionIds) {
		made.insertRule(motionRule(id), made.cssRules.length);
	}
	document.adoptedStyleSheets = [...document.adoptedStyleSheets, made];
	rules = new Map(
		motionIds.flatMap((id, index) => {
			const rule = made.cssRules.item(index);
			return rule instanceof CSSStyleRule ? [[id, rule] as const] : [];
		})
	);
	return rules;
}

/** Draws a column `px` from its slot, in every moving list. */
export function drawAt(id: MovableId, px: number): void {
	if ((offsets.get(id) ?? 0) === px) {
		return;
	}
	offsets.set(id, px);
	sheet()
		.get(id)
		?.style.setProperty('translate', `${String(px)}px 0`);
}

/** Where a column is drawn now, from its slot. */
export function offsetOf(id: MovableId): number {
	return offsets.get(id) ?? 0;
}

/** Holds a column's start back `ms`, for Reset's stagger. */
export function wait(id: MovableId, ms: number): void {
	if ((waits.get(id) ?? 0) === ms) {
		return;
	}
	waits.set(id, ms);
	sheet()
		.get(id)
		?.style.setProperty('transition-delay', `${String(ms)}ms`);
}

/** Every column back at its slot, with no wait. */
export function rest(): void {
	for (const id of [...offsets.keys()]) {
		drawAt(id, 0);
	}
	for (const id of [...waits.keys()]) {
		wait(id, 0);
	}
}

/** The columns drawn away from their slots now. */
export function moved(): MovableId[] {
	return [...offsets].flatMap(([id, px]) => (px === 0 ? [] : [id]));
}
