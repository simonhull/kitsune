<script lang="ts" module>
	/** The heart, for a control that draws it filled and outlined at once, to move between them. */
	export const heartPath =
		'M12 21s-7-4.5-9.5-9A5.5 5.5 0 0 1 12 6a5.5 5.5 0 0 1 9.5 6C19 16.5 12 21 12 21z';
</script>

<script lang="ts">
	const glyphs = {
		fox: { box: '0 0 32 32', path: 'M4 4 L12 12 L20 12 L28 4 L26 18 L16 28 L6 18 Z' },
		previous: { box: '0 0 24 24', path: 'M6 5h2v14H6zM20 5v14L9 12z' },
		next: { box: '0 0 24 24', path: 'M16 5h2v14h-2zM4 5v14l11-7z' },
		pause: { box: '0 0 24 24', path: 'M7 5h4v14H7zM13 5h4v14h-4z' },
		play: { box: '0 0 24 24', path: 'M7 4v16l13-8z' },
		heart: { box: '0 0 24 24', path: heartPath }
	} as const;

	const outlines = {
		search: { path: 'm20 20-3.5-3.5', circle: true },
		'chevron-down': { path: 'M6 9l6 6 6-6', circle: false },
		close: { path: 'M6 6l12 12M18 6L6 18', circle: false },
		'chevron-up': { path: 'M6 15l6-6 6 6', circle: false },
		columns: { path: 'M3 5h18v14H3zM9 5v14M15 5v14', circle: false },
		// The Shuffle board's: two crossing paths, an arrow at each end.
		shuffle: {
			path: 'M3 7h3.5c2 0 3.2 1 4.3 2.6l2.4 3.8c1.1 1.6 2.3 2.6 4.3 2.6H21M3 17h3.5c1.4 0 2.4-.5 3.2-1.3M14.3 8.3c.8-.8 1.8-1.3 3.2-1.3H21M18 4l3 3-3 3M18 14l3 3-3 3',
			circle: false
		}
	} as const;

	interface Props {
		name: keyof typeof glyphs | keyof typeof outlines;
		size?: number;
	}

	let { name, size = 18 }: Props = $props();
</script>

{#if name in glyphs}
	{@const glyph = glyphs[name as keyof typeof glyphs]}
	<svg width={size} height={size} viewBox={glyph.box} fill="currentColor" aria-hidden="true">
		<path d={glyph.path}></path>
	</svg>
{:else}
	{@const outline = outlines[name as keyof typeof outlines]}
	<svg
		width={size}
		height={size}
		viewBox="0 0 24 24"
		fill="none"
		stroke="currentColor"
		stroke-width="2"
		stroke-linecap={name === 'shuffle' ? 'round' : undefined}
		stroke-linejoin={name === 'shuffle' ? 'round' : undefined}
		aria-hidden="true"
	>
		{#if outline.circle}<circle cx="11" cy="11" r="7"></circle>{/if}
		<path d={outline.path}></path>
	</svg>
{/if}
