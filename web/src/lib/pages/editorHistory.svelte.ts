/**
 * An editor drawn over a page (the one tag editor, which the shell hosts) gets a history entry of its
 * own, a shallow one holding which editor it is. Back, from the browser, a mouse button or a gesture,
 * closes the editor and drops its edits, as Leave it does, and the page stays. Closing it any other way
 * (Leave it, Write, an apply) takes the entry back off, so a later Back goes where it would have gone
 * before the editor opened. An entry whose editor has gone, reached by Forward or a reload, goes back
 * off as well: nothing reopens edits that were dropped. Leaving the page drops them too.
 *
 * The entry is made with `pushState`, not a shallow `goto`: a shallow `goto` runs the navigation hooks,
 * and an editor opening is not a navigation for the morphs or for a newer build's reload.
 */
import { onDestroy, tick, untrack } from 'svelte';
import { pushState, replaceState } from '$app/navigation';
import { page } from '$app/state';

/**
 * The entries live editors have pushed and not yet taken off. More than one editor may watch one page
 * (the shell's tag editor, and a smart playlist's rules editor): each leaves the others' entries
 * alone, and only an entry nobody holds, reached by Forward or a reload, is taken back off.
 */
// eslint-disable-next-line svelte/prefer-svelte-reactivity -- Read inside effects only, never to draw.
const claimed = new Set<string>();
/** An entry being taken off, by whichever editor saw it first: the others wait for it. */
let offing: Promise<void> | null = null;

/** How an editor came to be closed by history: by Back, or by leaving the page with it open. */
export type Closed = 'back' | 'left';

export interface EditorHistory {
	/** Resolves once the entry of an editor just closed has come off, so a navigation can follow. */
	settled(): Promise<void>;
}

/**
 * Keeps the open editor's entry in step. `open` names the editor drawn now, or null; `close` drops
 * that editor's edits when Back took its entry, or when the page was left with it open. Call it while
 * a component initializes.
 */
export function editorHistory(
	open: () => string | null,
	close: (editor: string, how: Closed) => void
): EditorHistory {
	/**
	 * The editor whose entry this page added, on which path, and whether the page's state has shown
	 * it yet: `pushState` applies it a moment later, and only a state that held it can lose it to Back.
	 */
	let pushed: { editor: string; path: string; shown: boolean } | null = null;
	/** The entry this page is taking off, which a navigation waits for. */
	let leaving: Promise<void> | null = null;

	function takeOff(): void {
		leaving = new Promise((done) => {
			window.addEventListener(
				'popstate',
				() => {
					leaving = null;
					offing = null;
					done();
				},
				{ once: true }
			);
		});
		offing = leaving;
		history.back();
	}

	/** Lets go of the entry this page pushed, in the page's state and among those claimed. */
	function release(): void {
		if (pushed !== null) {
			claimed.delete(pushed.editor);
			pushed = null;
		}
	}

	$effect(() => {
		const editor = open();
		const path = page.url.pathname;
		const held = page.state.editor;
		untrack(() => {
			if (pushed !== null && pushed.path !== path) {
				// Left by a link: the entry stays behind it, and the edits go.
				const was = pushed.editor;
				release();
				close(was, 'left');
				// What this run read of the editor is from before it closed: the next run has it.
				return;
			}
			if (pushed !== null && held === pushed.editor) {
				pushed.shown = true;
			} else if (pushed?.shown === true) {
				// Back took the entry: close as Leave it does, which a field has left first.
				const was = pushed.editor;
				release();
				if (document.activeElement instanceof HTMLElement) {
					document.activeElement.blur();
				}
				close(was, 'back');
				return;
			}
			if (leaving !== null) {
				return;
			}
			if (editor === null) {
				if (pushed !== null) {
					release();
					takeOff();
				} else if (held !== undefined && !claimed.has(held) && offing === null) {
					takeOff(); // Forward or a reload onto an editor that has since closed.
				}
			} else if (pushed === null) {
				pushed = { editor, path, shown: false };
				claimed.add(editor);
				void pushState('', { editor });
			} else if (pushed.editor !== editor) {
				// One editor for another, as the match sheet's Edit by hand can open: the same entry.
				claimed.delete(pushed.editor);
				pushed = { editor, path, shown: false };
				claimed.add(editor);
				void replaceState('', { editor });
			}
		});
	});

	onDestroy(() => {
		if (pushed !== null) {
			const was = pushed.editor;
			release();
			close(was, 'left');
		}
	});

	return {
		async settled() {
			await tick();
			await leaving;
		}
	};
}
