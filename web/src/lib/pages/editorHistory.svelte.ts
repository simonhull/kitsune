/**
 * An editor drawn over a page (the album's tag editor, an Edit N songs) gets a history entry of its
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
					done();
				},
				{ once: true }
			);
		});
		history.back();
	}

	$effect(() => {
		const editor = open();
		const path = page.url.pathname;
		const held = page.state.editor;
		untrack(() => {
			if (pushed !== null && pushed.path !== path) {
				// Left by a link: the entry stays behind it, and the edits go.
				const was = pushed.editor;
				pushed = null;
				close(was, 'left');
			}
			if (pushed !== null && held === pushed.editor) {
				pushed.shown = true;
			} else if (pushed?.shown === true) {
				// Back took the entry: close as Leave it does, which a field has left first.
				const was = pushed.editor;
				pushed = null;
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
					pushed = null;
					takeOff();
				} else if (held !== undefined) {
					takeOff(); // Forward or a reload onto an editor that has since closed.
				}
			} else if (pushed === null) {
				pushed = { editor, path, shown: false };
				void pushState('', { editor });
			} else if (pushed.editor !== editor) {
				// One editor for another, as Many at once does: the same entry.
				pushed = { editor, path, shown: false };
				void replaceState('', { editor });
			}
		});
	});

	onDestroy(() => {
		if (pushed !== null) {
			close(pushed.editor, 'left');
			pushed = null;
		}
	});

	return {
		async settled() {
			await tick();
			await leaving;
		}
	};
}
