/** What already answers to Space, or wants the keystroke for itself. */
const claimsSpace = 'a[href], button, input, select, summary, textarea, [contenteditable]';

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
	return !(target instanceof Element && target.closest(claimsSpace) !== null);
}
