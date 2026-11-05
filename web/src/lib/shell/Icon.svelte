<script lang="ts">
	const glyphs = {
		fox: { box: '0 0 32 32', path: 'M4 4 L12 12 L20 12 L28 4 L26 18 L16 28 L6 18 Z' },
		previous: { box: '0 0 24 24', path: 'M6 5h2v14H6zM20 5v14L9 12z' },
		next: { box: '0 0 24 24', path: 'M16 5h2v14h-2zM4 5v14l11-7z' },
		pause: { box: '0 0 24 24', path: 'M7 5h4v14H7zM13 5h4v14h-4z' },
		play: { box: '0 0 24 24', path: 'M7 4v16l13-8z' },
		heart: {
			box: '0 0 24 24',
			path: 'M12 21s-7-4.5-9.5-9A5.5 5.5 0 0 1 12 6a5.5 5.5 0 0 1 9.5 6C19 16.5 12 21 12 21z'
		}
	} as const;

	const outlines = {
		search: { path: 'm20 20-3.5-3.5', circle: true },
		'chevron-down': { path: 'M6 9l6 6 6-6', circle: false },
		close: { path: 'M6 6l12 12M18 6L6 18', circle: false },
		'heart-outline': { path: glyphs.heart.path, circle: false }
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
		aria-hidden="true"
	>
		{#if outline.circle}<circle cx="11" cy="11" r="7"></circle>{/if}
		<path d={outline.path}></path>
	</svg>
{/if}
