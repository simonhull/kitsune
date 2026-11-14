/** What already answers to Space, or wants the keystroke for itself. */
const claimsSpace = 'a[href], button, input, select, summary, textarea, [contenteditable]';

/** Where a letter or a digit is typing, or picks an option. */
const claimsLetters = 'input, select, textarea, [contenteditable]';

/**
 * What uses ← and → itself: a field, a slider, a list of options, a menu, tabs, a toolbar (the
 * player's own), a grid, or a splitter such as a column's divider.
 */
const claimsArrows = [
	claimsLetters,
	...[
		'slider',
		'spinbutton',
		'radiogroup',
		'radio',
		'listbox',
		'option',
		'menu',
		'menubar',
		'menuitem',
		'menuitemradio',
		'menuitemcheckbox',
		'tablist',
		'tab',
		'toolbar',
		'grid',
		'gridcell',
		'tree',
		'treeitem',
		'separator'
	].map((role) => `[role="${role}"]`)
].join(', ');

/** How far ← and → seek, in seconds. */
export const seekStepSeconds = 10;

/** A shortcut the player answers anywhere on the page. */
export type PlayerKey =
	| { kind: 'toggle' }
	| { kind: 'seek'; by: number }
	| { kind: 'previous' }
	| { kind: 'next' }
	| { kind: 'shuffle' }
	| { kind: 'repeat' }
	/** Rates the song playing: 1 to 5, or null to clear it. */
	| { kind: 'rate'; stars: number | null }
	| { kind: 'now-playing' };

function claimed(target: EventTarget | null, selector: string): boolean {
	return target instanceof Element && target.closest(selector) !== null;
}

/** Reports whether a key press is Space with nothing else to do. */
export function togglesOnSpace({
	code,
	repeat,
	altKey,
	ctrlKey,
	metaKey,
	shiftKey,
	target
}: KeyboardEvent): boolean {
	if (code !== 'Space' || repeat || altKey || ctrlKey || metaKey || shiftKey) {
		return false;
	}
	return !claimed(target, claimsSpace);
}

/** ← and →: a seek, or with Shift the song before or after, unless something focused uses them. */
function arrowKey(event: KeyboardEvent): PlayerKey | null {
	const back = event.key === 'ArrowLeft';
	if ((!back && event.key !== 'ArrowRight') || claimed(event.target, claimsArrows)) {
		return null;
	}
	if (event.shiftKey) {
		// A held Shift+arrow would skip through the queue faster than anyone could hear it.
		if (event.repeat) {
			return null;
		}
		return { kind: back ? 'previous' : 'next' };
	}
	// Seeking is the one shortcut a held key repeats.
	return { kind: 'seek', by: back ? -seekStepSeconds : seekStepSeconds };
}

/** S, R, N and the digits: never in a field, and never held. */
function letterKey(event: KeyboardEvent): PlayerKey | null {
	if (event.repeat || event.shiftKey || claimed(event.target, claimsLetters)) {
		return null;
	}
	switch (event.key.toLowerCase()) {
		case 's':
			return { kind: 'shuffle' };
		case 'r':
			return { kind: 'repeat' };
		case 'n':
			return { kind: 'now-playing' };
		case '0':
			return { kind: 'rate', stars: null };
		default:
			return /^[1-5]$/.test(event.key) ? { kind: 'rate', stars: Number(event.key) } : null;
	}
}

/**
 * The player's shortcut a key press is, if any: Space plays or pauses; ← → seek 10 s; Shift+← →
 * go to the song before or after; S shuffles; R moves Repeat on; 1 to 5 rate the song playing and 0
 * clears it; N opens Now playing, and closes it again. None in a text field, with a modifier other
 * than Shift, or held down (seeking aside), and none that something focused has already taken.
 */
export function playerKey(event: KeyboardEvent): PlayerKey | null {
	if (event.defaultPrevented || event.altKey || event.ctrlKey || event.metaKey) {
		return null;
	}
	if (togglesOnSpace(event)) {
		return { kind: 'toggle' };
	}
	return arrowKey(event) ?? letterKey(event);
}
