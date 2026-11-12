<script lang="ts">
	import { library } from '#lib/library/library.svelte.ts';
	import Icon from '#lib/shell/Icon.svelte';
	import { shuffles, type ShuffleSource } from '#lib/shuffle/session.svelte.ts';

	interface Props {
		/** What a press shuffles. */
		source: ShuffleSource;
		/** `system` is the amber tile, the frame's own; `card` is a quiet one with the fox heart. */
		look: 'system' | 'card';
		/** In large type, after "Shuffle". `short` is what a phone says. */
		title: string;
		short: string;
		note: string;
		/** Names the tile for the page's arrivals. */
		morphKey: string;
	}

	let { source, look, title, short, note, morphKey }: Props = $props();

	/**
	 * Gets the first song ready ahead of a press, as the Library's Shuffle does: it's picked from
	 * every song, so the songs are asked for first if this browser hasn't read them yet.
	 */
	function prime(): void {
		void (async () => {
			if (library.index.kind !== 'ready') {
				await library.loadIndex();
			}
			await shuffles.prime(source);
		})();
	}

	/**
	 * The song starts here, from what priming picked; everything after it is off the play path. A
	 * press with nothing primed picks once the songs are here.
	 */
	function shuffle(): void {
		if (library.index.kind === 'ready') {
			shuffles.start(source);
			void shuffles.prime(source);
			return;
		}
		void library.loadIndex().then(() => {
			shuffles.start(source);
		});
	}
</script>

<button
	type="button"
	class="tile {look}"
	data-morph-part="card"
	data-morph-key={morphKey}
	aria-label={`Shuffle ${title.toLowerCase()}`}
	onpointerenter={prime}
	onpointerdown={prime}
	onfocus={prime}
	onclick={shuffle}
>
	{#if look === 'system'}<span class="fox-mark"><Icon name="fox" size={230} /></span>{/if}
	<span class="label">Shuffle</span>
	<span class="glyph">
		{#if look === 'system'}<Icon name="shuffle" size={28} />{:else}<Icon
				name="heart"
				size={26}
			/>{/if}
	</span>
	<span class="title"
		><span class="long">{title}</span><span class="short" aria-hidden="true">{short}</span></span
	>
	<span class="note">{note}</span>
</button>

<style>
	.tile {
		position: relative;
		overflow: hidden;
		min-width: 0;
		border-radius: var(--radius-28);
		padding: calc(28px * var(--step));
		display: grid;
		grid-template:
			'label glyph' auto
			'. .' 1fr
			'title title' auto
			'note note' auto / 1fr auto;
		row-gap: calc(8px * var(--step));
		text-align: left;
		font: inherit;
	}

	.system {
		border: 0;
		background: var(--fox);
		color: var(--fox-ink);
	}

	.system:hover {
		background: color-mix(in srgb, var(--fox) 90%, var(--text));
	}

	.card {
		border: 1px solid var(--edge-card);
		background: var(--card);
		color: var(--text);
	}

	.card:hover {
		border-color: var(--edge-hover);
	}

	.tile:active {
		scale: 0.985;
	}

	.fox-mark {
		display: flex;
		position: absolute;
		right: calc(-40px * var(--step));
		bottom: calc(-60px * var(--step));
		opacity: 0.18;
	}

	.fox-mark :global(svg) {
		width: calc(230px * var(--step));
		height: calc(230px * var(--step));
	}

	.label,
	.glyph,
	.title,
	.note {
		position: relative;
	}

	.label {
		grid-area: label;
		align-self: center;
		font-size: calc(13px * var(--step));
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
	}

	.system .label {
		font-weight: 700;
	}

	.card .label {
		color: var(--text-meta);
	}

	.glyph {
		grid-area: glyph;
		display: flex;
	}

	.glyph :global(svg) {
		width: calc(28px * var(--step));
		height: calc(28px * var(--step));
	}

	.card .glyph {
		color: var(--fox);
	}

	.title {
		grid-area: title;
		font-size: calc(44px * var(--step));
		font-weight: 800;
		letter-spacing: -0.05em;
		line-height: 0.9;
	}

	.system .title {
		font-size: calc(52px * var(--step));
		font-weight: 900;
		line-height: 0.85;
	}

	.short {
		display: none;
	}

	.note {
		grid-area: note;
		font-family: var(--font-mono);
		font-size: calc(13px * var(--step));
	}

	.system .note {
		font-weight: 600;
	}

	.card .note {
		color: var(--text-secondary);
	}

	/* On a phone the two sit side by side: the glyph on top, the words at the foot, no note. */
	@media (max-width: 720px) {
		.tile {
			height: 168px;
			padding: 18px;
			border-radius: var(--radius-24);
			grid-template:
				'glyph' auto
				'.' 1fr
				'label' auto
				'title' auto / 1fr;
			row-gap: 6px;
		}

		.fox-mark {
			right: -36px;
			bottom: -48px;
		}

		.fox-mark :global(svg) {
			width: 150px;
			height: 150px;
		}

		.label {
			font-size: 12px;
		}

		.glyph :global(svg) {
			width: 24px;
			height: 24px;
		}

		.title,
		.system .title {
			font-size: 28px;
			line-height: 0.9;
		}

		.long,
		.note {
			display: none;
		}

		.short {
			display: inline;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.tile:active {
			scale: 1;
		}
	}
</style>
