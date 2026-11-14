<!--
	Where the shell draws the one tag editor, over whatever page opened it. The editor has a history
	entry of its own (`editorHistory`): Back is Leave it, and leaving the page drops its edits.
-->
<script lang="ts">
	import { tick } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { library } from '#lib/library/library.svelte.ts';
	import { setMatchState } from '#lib/matching/api.ts';
	import { matchSheet } from '#lib/matching/sheet.svelte.ts';
	import { editorHistory } from './editorHistory.svelte.ts';
	import TagEditor from './TagEditor.svelte';
	import { tagEditor, type Written } from './tagEditor.svelte.ts';

	let announcement = $state('');

	const editors = editorHistory(
		() => tagEditor.current?.key ?? null,
		(editor) => {
			if (tagEditor.current?.key === editor) {
				tagEditor.close();
			}
		}
	);

	function leave(): void {
		announcement = '';
		tagEditor.close();
	}

	/**
	 * Closes a written editor. An edit that moved every song off the album page's album took the album
	 * with it: the page follows them there.
	 */
	async function done({ albumId, matched }: Extract<Written, { kind: 'done' }>): Promise<void> {
		tagEditor.close();
		if (matched !== null) {
			matchSheet.forget(matched);
			// The queue's word for it: a failure here costs only the Matched tab its row.
			void setMatchState(library.movedTo(matched) ?? matched, 'matched').catch(() => undefined);
		}
		const here = page.route.id === '/album/[id]' ? page.params['id'] : undefined;
		const to = here === undefined ? undefined : (library.movedTo(here) ?? albumId);
		if (here !== undefined && to !== undefined && to !== here && library.failed(here)) {
			// The editor's entry comes off first, so the album gone takes its place, not the entry.
			await editors.settled();
			await goto(resolve('/album/[id]', { id: to }), { replaceState: true });
			return;
		}
		announcement = '';
		await tick();
		announcement = 'Tags written.';
	}
</script>

{#if tagEditor.current !== null}
	{#key tagEditor.current.key}
		<TagEditor session={tagEditor.current} {leave} done={(written) => void done(written)} />
	{/key}
{/if}
<p class="visually-hidden" role="status">{announcement}</p>
