<!--
	Three of the library's songs as the template being typed would file them, worked out in the page
	with the client's copy of the parser: each one's path now, struck where it changes, and the new
	one in the fox. A template that doesn't parse leaves the last one that did.
-->
<script lang="ts">
	import { extensionOf, pathOf, type Template } from './template.ts';
	import type { OrganizeExample } from './types.ts';
	import { pathDiff } from './words.ts';

	interface Props {
		examples: readonly OrganizeExample[];
		template: Template | null;
		compilations: Template | null;
		/** One song at a time, on a phone. */
		one: boolean;
		onanother: () => void;
	}

	let { examples, template, compilations, one, onanother }: Props = $props();

	const rows = $derived(
		(one ? examples.slice(0, 1) : examples).map((example) => {
			const t = example.song.compilation ? compilations : template;
			const to = t === null ? null : pathOf(t, example.song, extensionOf(example.from));
			const path = to === null || to === 'noTags' || to === 'tooLong' ? null : to;
			const same = path === example.from;
			const dirOf = (p: string): string => p.slice(0, Math.max(0, p.lastIndexOf('/')));
			const chip =
				path === null
					? 'SKIPPED'
					: same
						? 'IN PLACE'
						: dirOf(path) === dirOf(example.from)
							? 'RENAME'
							: 'NEW FOLDER';
			const diff = path === null ? null : pathDiff(example.from, path);
			const title = example.title.length > 46 ? `${example.title.slice(0, 44)}…` : example.title;
			return { example, path, same, chip, diff, title };
		})
	);
</script>

<section class="example" aria-label="Example">
	<div class="top">
		<span class="kicker mono"
			>{one ? 'ONE OF YOUR SONGS' : 'THREE OF YOUR SONGS'}, AS IT WOULD FILE {one
				? 'IT'
				: 'THEM'}</span
		>
		<button type="button" class="another" onclick={onanother}>Try another song</button>
	</div>
	{#each rows as row (row.example.track)}
		<div class="song" data-testid="example">
			<div class="who">
				<span class="title">{row.title}</span>
				<span class="by mono"
					>{row.example.artist.toUpperCase()} · {row.example.album.toUpperCase().slice(0, 34)}</span
				>
				<span class="chip mono" class:quiet={row.same}>{row.chip}</span>
			</div>
			<div class="path old mono">
				{#if row.diff === null}{row.example.from}{:else}{#each row.diff.old as part, i (i)}<span
							class:struck={!row.same && part.changed}>{part.text}</span
						>{/each}{/if}
			</div>
			{#if row.diff !== null && !row.same}
				<div class="path new mono" data-testid="example-new">
					<span class="arrow">→ </span>{#each row.diff.new as part, i (i)}<span
							class:changed={part.changed}>{part.text}</span
						>{/each}
				</div>
			{/if}
		</div>
	{/each}
</section>

<style>
	.example {
		display: flex;
		flex-direction: column;
		gap: 4px;
		padding: calc(22px * var(--step)) calc(24px * var(--step));
		border-radius: var(--radius-20);
		background: var(--card);
		border: 1px solid var(--edge);
	}

	.mono {
		font-family: var(--font-mono);
	}

	.top {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
		margin-bottom: 8px;
	}

	.kicker {
		font-size: calc(11px * var(--step));
		letter-spacing: 0.1em;
		color: var(--text-meta);
	}

	.another {
		flex: none;
		height: calc(32px * var(--step));
		padding: 0 12px;
		border-radius: var(--radius-pill);
		border: 1px solid var(--edge-strong);
		background: none;
		color: var(--text);
		font: 600 calc(13px * var(--step)) var(--font-sans);
		cursor: pointer;
	}

	.another:hover {
		background: var(--hover);
	}

	.song {
		display: flex;
		flex-direction: column;
		gap: 6px;
		padding: calc(14px * var(--step)) 0;
		border-top: 1px solid var(--edge);
	}

	.who {
		display: flex;
		align-items: center;
		gap: 10px;
		min-width: 0;
	}

	.title {
		font-size: calc(15px * var(--step));
		font-weight: 700;
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
	}

	.by {
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
		font-size: calc(11px * var(--step));
		color: var(--text-meta);
	}

	.chip {
		margin-left: auto;
		flex: none;
		padding: 3px 8px;
		border-radius: 6px;
		border: 1.5px dashed color-mix(in srgb, var(--fox-line) 55%, transparent);
		font-size: calc(10.5px * var(--step));
		font-weight: 700;
		letter-spacing: 0.06em;
		color: var(--fox-text);
		transform: rotate(-1deg);
	}

	.chip.quiet {
		border-color: var(--edge-strong);
		color: var(--text-meta);
	}

	.path {
		font-size: calc(12.5px * var(--step));
		line-height: 1.55;
		word-break: break-all;
	}

	.old {
		color: var(--text-meta);
	}

	.struck {
		text-decoration: line-through;
	}

	.new {
		color: var(--text);
	}

	.arrow {
		color: var(--fox-text);
	}

	.changed {
		color: var(--fox-text);
		font-weight: 700;
	}
</style>
