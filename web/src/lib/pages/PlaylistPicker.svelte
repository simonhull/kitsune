<!--
	Add to playlist, as a surface of its own: beside a menu's item a submenu, born at the item's chevron;
	above the selection bar a popover, born at Add to…. On a phone there is no room beside a menu, so the
	menu itself reshapes into the list (`PlaylistList`), never a sheet.
-->
<script lang="ts">
	import { untrack } from 'svelte';
	import type { Rect } from '#lib/morph/bud.ts';
	import Floating from './Floating.svelte';
	import PlaylistList from './PlaylistList.svelte';

	interface Props {
		trackIds: readonly string[];
		resolveTracks?: (() => Promise<readonly string[]>) | undefined;
		count: number;
		mode: 'submenu' | 'popover';
		/** Where it opens: beside the item, or (`popover`) the bottom-left corner over the button. */
		x?: number;
		y?: number;
		/** Where it is born: the item's chevron, for a submenu. */
		from?: Rect | undefined;
		heading?: string | undefined;
		onclose: (done: boolean, refocus: boolean) => void;
	}

	let {
		trackIds,
		resolveTracks,
		count,
		mode,
		x = 0,
		y = 0,
		from,
		heading,
		onclose
	}: Props = $props();

	// Where and how it opened, kept as it was: closing takes away what opened it.
	const at = untrack(() => ({ mode, x, y, from }));
</script>

<Floating
	x={at.x}
	y={at.y}
	label="Add to playlist"
	width={300}
	submenu={at.mode === 'submenu'}
	above={at.mode === 'popover'}
	from={at.from}
	onclose={(refocus) => {
		onclose(false, refocus);
	}}
>
	<PlaylistList {trackIds} {resolveTracks} {count} {heading} {onclose} />
</Floating>
