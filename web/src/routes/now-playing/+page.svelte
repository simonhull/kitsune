<script lang="ts">
	import { resolve } from '$app/paths';
	import Cover from '#lib/cover/Cover.svelte';
	import { emWidth } from '#lib/library/fit.ts';
	import { balancedLines, credit, gain, trackGain } from '#lib/library/format.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import { goUpToAlbum, hasInAppHistory } from '#lib/morph/journey.ts';
	import { morph } from '#lib/morph/registry.ts';
	import Icon from '#lib/shell/Icon.svelte';
	import { reserveIslandSlot } from '#lib/shell/slot.svelte.ts';

	const playing = $derived(library.playing);
	const lines = $derived(playing === null ? ['', ''] : balancedLines(playing.track.title));
	const longest = $derived(Math.max(...lines.map((line) => emWidth(line, 900, -0.075)), 1));
	const others = $derived(
		playing === null ? undefined : credit(playing.track, playing.album.artist)
	);
	const rg = $derived(playing === null ? null : trackGain(playing.track));

	function goUp(event: MouseEvent & { currentTarget: HTMLAnchorElement }): void {
		goUpToAlbum(event, event.currentTarget.pathname);
	}

	function close(event: MouseEvent): void {
		if (hasInAppHistory()) {
			event.preventDefault();
			history.back();
		}
	}
</script>

<svelte:head>
	<title>{playing?.track.title ?? 'Now playing'} · Kitsune</title>
</svelte:head>

{#if playing === null}
	<!-- Plan 018 replaces this: with real playback there's always a last thing played. -->
	<div class="stage quiet">
		<p>Nothing is cued yet.</p>
		<a href={resolve('/')} data-sveltekit-reset="false">Back to Home</a>
	</div>
{:else}
	<div class="stage">
		<div class="column">
			<div class="top" data-morph-part="chrome">
				<a
					class="close"
					href={resolve('/album/[id]', { id: playing.album.id })}
					aria-label="Close now playing"
					onclick={close}><Icon name="chevron-down" size={20} /></a
				>
				<span class="where">
					NOW PLAYING ·
					<a
						class="album-link"
						href={resolve('/album/[id]', { id: playing.album.id })}
						aria-label={`Go to the album ${playing.album.title}`}
						data-sveltekit-replacestate
						onclick={goUp}>{playing.album.title.toUpperCase()}</a
					>
					· {playing.index + 1} / {playing.trackCount}
				</span>
				<span class="chips">
					<span class="chip">{playing.format}</span>
					{#if rg !== null}
						<span class="chip">RG {gain(rg)} dB</span>
					{/if}
				</span>
			</div>

			<div class="center">
				<div class="stack">
					<div class="poster" aria-hidden="true" style:--width={longest}>
						{#each lines as line, index (index)}
							<span data-morph-part={`line-${String(index + 1)}`}>{line}</span>
						{/each}
					</div>
					<a
						class="art"
						href={resolve('/album/[id]', { id: playing.album.id })}
						aria-label={`Go to the album ${playing.album.title}`}
						data-sveltekit-replacestate
						onclick={goUp}
						use:morph={{ id: `cover:${playing.album.id}`, slot: 'poster', kind: 'cover' }}
					>
						<Cover albumId={playing.album.id} entry={playing.album} place="stage" />
					</a>
				</div>
				<div class="names">
					<h1 use:morph={{ id: 'song', slot: 'poster', kind: 'title' }}>{playing.track.title}</h1>
					<span class="artist" data-morph-part="details">
						<strong>{playing.album.artist}</strong>
						{#if others !== undefined}{others}{/if}
					</span>
				</div>
				<div class="controls-slot" use:reserveIslandSlot></div>
			</div>

			{#if playing.upNext.length > 0}
				<section aria-label="Up next" class="next" data-morph-part="panel">
					<span class="next-label">UP NEXT</span>
					{#each playing.upNext as upcoming, index (upcoming.id)}
						<span class="pill" style:opacity={1 - index * 0.2}>
							<span class="dot"
								><Cover albumId={playing.album.id} entry={playing.album} place="small" /></span
							>
							<span class="pill-name" class:lead={index === 0}>{upcoming.title}</span>
						</span>
					{/each}
					{#if playing.moreCount > 0}<span class="more">+{playing.moreCount} more</span>{/if}
				</section>
			{/if}
		</div>
	</div>
{/if}

<style>
	.stage {
		container-type: inline-size;
		position: relative;
		min-height: 100dvh;
		overflow: hidden;
	}

	.quiet {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 8px;
		padding: 16px;
		text-align: center;
		color: var(--text-secondary);
	}

	.quiet a {
		min-height: var(--target);
		display: flex;
		align-items: center;
		color: var(--fox);
	}

	/* The cover and the title behind it share a center, so they can't drift apart with height. */
	.stack {
		position: relative;
		display: flex;
		justify-content: center;
		width: 100%;
	}

	.poster {
		position: absolute;
		left: 50%;
		top: 50%;
		transform: translate(-50%, -50%);
		display: flex;
		flex-direction: column;
		align-items: center;
		font-size: min(230px, 25.6dvh, calc((100cqw - 32px) / var(--width)));
		font-weight: 900;
		letter-spacing: -0.075em;
		line-height: 0.8;
		color: var(--accent);
		white-space: nowrap;
	}

	.column {
		position: relative;
		min-height: 100dvh;
		padding: 24px 32px;
		display: flex;
		flex-direction: column;
	}

	.top {
		position: relative;
		z-index: 1;
		display: flex;
		align-items: center;
		gap: 12px;
	}

	.close {
		flex: none;
		width: var(--target);
		height: var(--target);
		border-radius: 50%;
		background: rgb(10 9 8 / 30%);
		border: 1px solid rgb(255 255 255 / 12%);
		backdrop-filter: blur(20px);
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.where,
	.chips,
	.next-label {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		color: var(--wash);
	}

	.where {
		display: flex;
		align-items: center;
		gap: 0.6em;
		min-width: 0;
		letter-spacing: 0.06em;
		white-space: nowrap;
	}

	/* The words stay small; the target doesn't. */
	.album-link {
		min-width: var(--target);
		min-height: var(--target);
		padding: 0 6px;
		margin: 0 -6px;
		display: block;
		overflow: hidden;
		line-height: var(--target);
		text-align: center;
		text-overflow: ellipsis;
		text-underline-offset: 3px;
	}

	.album-link:hover {
		text-decoration: underline;
	}

	.album-link:focus-visible {
		outline-offset: -4px;
	}

	.chips {
		flex: none;
		margin-left: auto;
		display: flex;
		gap: 8px;
	}

	.chip {
		padding: 6px 12px;
		border-radius: var(--radius-pill);
		background: rgb(10 9 8 / 30%);
		border: 1px solid rgb(255 255 255 / 12%);
	}

	.center {
		flex: 1 1 auto;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 28px;
		padding: 24px 0 8px;
	}

	/* A link with no hover of its own: the art isn't a button, and nothing should fight it. */
	.art {
		position: relative;
		display: block;
		width: min(400px, 46dvh, 100%);
		aspect-ratio: 1;
		border-radius: var(--radius-28);
		overflow: hidden;
		box-shadow:
			0 60px 120px rgb(0 0 0 / 55%),
			0 0 0 1px rgb(255 255 255 / 8%);
	}

	/* Positioned, so the cover's long shadow falls behind the words and not across them. */
	.names {
		position: relative;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 4px;
		text-align: center;
	}

	h1 {
		display: -webkit-box;
		-webkit-box-orient: vertical;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		overflow: hidden;
		max-width: min(640px, 100cqw - 32px);
		font-size: 28px;
		font-weight: 800;
		letter-spacing: -0.03em;
		line-height: 1.15;
		text-wrap: balance;
	}

	.artist {
		font-size: 16px;
		color: var(--wash);
		max-width: min(640px, 100cqw - 32px);
	}

	.artist strong {
		font-weight: 600;
		color: var(--text);
	}

	.controls-slot {
		width: min(640px, 100%);
		height: 90px;
	}

	.next {
		display: flex;
		align-items: flex-end;
		justify-content: center;
		gap: 14px;
	}

	.next-label {
		white-space: nowrap;
		margin-right: 6px;
		align-self: center;
	}

	.pill {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 6px 14px 6px 6px;
		border-radius: var(--radius-pill);
		background: rgb(10 9 8 / 35%);
		border: 1px solid rgb(255 255 255 / 10%);
	}

	.dot {
		position: relative;
		flex: none;
		width: 36px;
		height: 36px;
		border-radius: 50%;
		overflow: hidden;
	}

	.pill {
		min-width: 0;
	}

	.pill-name {
		overflow: hidden;
		font-size: 14px;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.lead {
		font-weight: 600;
	}

	.more {
		height: 48px;
		padding: 0 16px;
		display: flex;
		align-items: center;
		font-size: 14px;
		color: var(--wash);
	}

	@media (max-width: 720px) {
		.column {
			padding: 16px;
		}

		/* Too narrow to frame the cover, so the title stands on it, tucked in by a fixed amount. */
		.stack {
			flex-direction: column;
			align-items: center;
		}

		.poster {
			position: static;
			transform: none;
			margin-bottom: -16px;
			font-size: min(11dvh, calc((100cqw - 32px) / var(--width)));
		}

		.chips {
			display: none;
		}

		.art {
			width: min(280px, 33dvh, 100%);
		}

		.center {
			gap: 18px;
		}

		.controls-slot {
			height: 126px;
		}

		.next {
			flex-wrap: nowrap;
			justify-content: flex-start;
			overflow-x: auto;
			scrollbar-width: none;
			margin: 0 -16px;
			padding: 0 16px;
		}

		.pill {
			flex: none;
		}

		.more {
			flex: none;
		}
	}

	@media (max-width: 720px) and (max-height: 700px) {
		.art {
			width: min(280px, 28dvh, 100%);
		}

		.center {
			gap: 12px;
			padding: 8px 0 4px;
		}
	}
</style>
