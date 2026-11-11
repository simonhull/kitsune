<!--
	The You page's Hearts card: what a hearted song with no stars of its own counts as, Off or 1★ to
	5★ in halves (`ratings.heart_stars`). The control is Off and the same slider a song is rated with,
	held at 1★ and up; a sample row shows the look as it's chosen.
-->
<script lang="ts">
	import { untrack } from 'svelte';
	import { heartStarsOff, heartStarsValue, leastHeartStars } from '#lib/library/heartStars.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import { starsText } from '#lib/library/stars.ts';
	import { heartPath } from '#lib/shell/Icon.svelte';
	import StarsInput from '#lib/shell/StarsInput.svelte';

	const id = $props.id();
	const heartStars = $derived(library.marks.heartStars);
	const off = $derived(heartStars === heartStarsOff);

	/**
	 * The sample: the first hearted song with no stars of its own in the Library, so the look is shown
	 * on one of the person's; a plain name when there's none to hand. Looked for once per index, with
	 * the marks read untracked, so a big library doesn't hang every mark on this card.
	 */
	const sample = $derived.by(() => {
		const index = library.index.kind === 'ready' ? library.index.value : null;
		return untrack(() => {
			for (const album of index?.artists.flatMap(({ albums }) => albums) ?? []) {
				const found = album.tracks.find(
					({ id }) => library.marks.hearted('tracks', id) && library.marks.stars(id) === null
				);
				if (found !== undefined) {
					return found.title;
				}
			}
			return 'A song you hearted';
		});
	});

	/** A rating set on the slider: clearing it is Off, and under 1★ is 1★. */
	function choose(stars: number | null): void {
		library.setHeartStars(stars === null ? heartStarsOff : Math.max(leastHeartStars, stars));
	}
</script>

<section aria-label="Hearts" class="card">
	<h3>Hearts</h3>

	<div class="setting">
		<div class="top">
			<span class="label" id={`${id}-label`}>A heart counts as</span>
			<span class="value">{heartStarsValue(heartStars)}</span>
		</div>
		<div class="worth" class:off>
			<button
				type="button"
				class="off-switch"
				role="switch"
				aria-checked={off}
				aria-label="Hearts count as nothing"
				onclick={() => {
					library.setHeartStars(heartStarsOff);
				}}>Off</button
			>
			<StarsInput
				size="setting"
				value={off ? null : heartStars}
				label="A heart counts as"
				aria-describedby={`${id}-note`}
				onchange={choose}
			/>
		</div>
		<p class="note" id={`${id}-note`} aria-live="polite">
			{#if off}
				A heart is just a heart. A hearted song with no stars counts as unrated, as it always has.
			{:else}
				A song you’ve hearted but never rated counts as {starsText(heartStars)}★: in ★ 4 and up, Top
				songs, sorting by rating, album averages and shuffle. Nothing is written to the song. Stars
				you set always win, and these show a heart in their stars until you do.
			{/if}
		</p>
	</div>

	<!-- Only a picture of the look: nothing in it is read out or takes a press. -->
	<div class="sample" aria-hidden="true" inert>
		<svg class="lead-heart" viewBox="0 0 24 24"><path d={heartPath}></path></svg>
		<span class="title">{sample}</span>
		<StarsInput value={off ? null : heartStars} byHeart={!off} label="Sample" />
		<span class="tag">{off ? 'UNRATED' : 'BY HEART'}</span>
	</div>
</section>

<style>
	.card {
		margin-top: 16px;
		padding: 32px;
		display: flex;
		flex-direction: column;
		gap: 22px;
		border-radius: var(--radius-28);
		background: var(--card);
		border: 1px solid var(--edge-card);
	}

	h3 {
		margin: 0;
		font-size: var(--type-section);
		font-weight: 800;
		letter-spacing: var(--tracking-section);
		line-height: 1;
	}

	.setting {
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	.top {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 12px;
	}

	.label {
		font-size: 15px;
		font-weight: 600;
	}

	.value,
	.tag {
		font-family: var(--font-mono);
		letter-spacing: 0.06em;
		white-space: nowrap;
	}

	.value {
		font-size: 12px;
		color: var(--fox-text);
	}

	/* Off, and the stars: one pill, so the two read as one choice. */
	.worth {
		display: flex;
		align-items: center;
		gap: 10px;
		width: fit-content;
		max-width: 100%;
		padding: 4px 12px 4px 4px;
		border-radius: var(--radius-pill);
		background: var(--input);
		border: 1px solid var(--edge);
	}

	.off-switch {
		height: 36px;
		padding: 0 16px;
		border: 0;
		border-radius: var(--radius-pill);
		background: transparent;
		color: var(--text-secondary);
		font-size: 13px;
		font-weight: 600;
	}

	.off-switch[aria-checked='true'] {
		background: var(--fox);
		color: var(--fox-ink);
		font-weight: 700;
	}

	.off-switch:focus-visible {
		outline: 2px solid var(--fox-line);
		outline-offset: 2px;
	}

	.note {
		margin: 0;
		max-width: 62ch;
		font-size: 13px;
		line-height: 1.5;
		color: var(--text-meta);
	}

	.sample {
		display: flex;
		align-items: center;
		gap: 14px;
		padding: 12px 14px;
		border-radius: var(--radius-12);
		background: var(--input);
	}

	.lead-heart {
		flex: none;
		width: 14px;
		height: 14px;
		fill: var(--star);
	}

	.title {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		font-size: 14px;
		font-weight: 600;
		white-space: nowrap;
		text-overflow: ellipsis;
	}

	.tag {
		font-size: 11px;
		color: var(--text-label);
	}

	@media (max-width: 720px) {
		.card {
			padding: 22px 18px;
			gap: 20px;
		}

		.worth {
			width: 100%;
			justify-content: space-between;
			padding-right: 4px;
		}
	}
</style>
