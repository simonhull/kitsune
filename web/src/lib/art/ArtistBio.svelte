<!--
	The bio, under the shelves: a card with the summary and More. Its credit is mono and always
	shown, never behind the fold: where it came from, its license as a link, that it was shortened,
	and the way to the whole of it. A paper tag says it came from somewhere else, as a file
	playlist's does. More grows the card at its true height, the rest revealed by it; Less lets the
	rest give way where it stands and closes the card under it.
-->
<script lang="ts">
	import type { ArtistBio } from '#lib/library/source.ts';
	import { bioCredit, bioRest, bioTag } from './art.ts';

	interface Props {
		name: string;
		bio: ArtistBio;
	}

	let { name, bio }: Props = $props();

	let open = $state(false);
	const rest = $derived(bioRest(bio));
	const credit = $derived(bioCredit(bio));
</script>

<section class="bio" aria-label={`About ${name}`} data-morph-part="panel" data-morph-key="bio">
	<span class="tag" aria-hidden="true">{bioTag(bio.source)}</span>
	<h2 class="about">About {name}</h2>
	<p class="summary" lang={bio.lang}>{bio.summary}</p>
	{#if rest.length > 0}
		<div class="rest" class:open inert={!open} id="bio-rest">
			<div class="inner">
				{#each rest as paragraph, index (index)}
					<p lang={bio.lang}>{paragraph}</p>
				{/each}
			</div>
		</div>
	{/if}
	<p class="credit">
		{#each credit as part, index (index)}
			{#if index > 0}<span class="dot" aria-hidden="true">·</span>{/if}
			{#if part.href === undefined}
				<span>{part.text}</span>
			{:else}
				<!-- eslint-disable svelte/no-navigation-without-resolve -- The source's own page, not a route. -->
				<a
					href={part.href}
					target="_blank"
					rel="noreferrer noopener"
					class:read={index === credit.length - 1}>{part.text}</a
				>
				<!-- eslint-enable svelte/no-navigation-without-resolve -->
			{/if}
		{/each}
	</p>
	{#if rest.length > 0}
		<button
			type="button"
			class="more"
			aria-expanded={open}
			aria-controls="bio-rest"
			onclick={() => {
				open = !open;
			}}>{open ? 'Less' : 'More'}</button
		>
	{/if}
</section>

<style>
	.bio {
		position: relative;
		max-width: calc(780px * var(--step));
		padding: calc(32px * var(--step)) calc(34px * var(--step)) calc(32px * var(--step));
		border-radius: var(--radius-28);
		background: var(--card);
		box-shadow: inset 0 0 0 1px var(--cover-edge);
		display: flex;
		flex-direction: column;
		gap: calc(16px * var(--step));
	}

	/* The paper tag: it came from somewhere else. */
	.tag {
		position: absolute;
		top: calc(-12px * var(--step));
		right: calc(22px * var(--step));
		height: calc(30px * var(--step));
		padding: 0 calc(12px * var(--step));
		display: flex;
		align-items: center;
		border-radius: 3px;
		background: var(--paper);
		color: var(--paper-ink);
		box-shadow: 0 6px 14px color-mix(in srgb, var(--paper-ink) 35%, transparent);
		font-family: var(--font-mono);
		font-size: calc(11px * var(--step));
		font-weight: 700;
		letter-spacing: 0.12em;
		transform: rotate(-1.5deg);
	}

	.about {
		font-family: var(--font-mono);
		font-size: calc(12px * var(--step));
		font-weight: 500;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--text-meta);
	}

	.summary {
		font-size: calc(17px * var(--step));
		line-height: 1.55;
		color: var(--text);
		text-wrap: pretty;
	}

	.rest {
		display: grid;
		grid-template-rows: 0fr;
		margin-top: calc(-16px * var(--step));
		transition:
			grid-template-rows var(--morph) var(--curve-settle),
			margin-top var(--morph) var(--curve-settle);
	}

	.rest.open {
		grid-template-rows: 1fr;
		margin-top: 0;
	}

	.inner {
		min-height: 0;
		overflow: hidden;
		display: flex;
		flex-direction: column;
		gap: calc(16px * var(--step));
		opacity: 0;
		transition: opacity var(--morph-fast) var(--curve-morph);
	}

	.open .inner {
		opacity: 1;
		transition-delay: 80ms;
	}

	.inner p {
		font-size: calc(17px * var(--step));
		line-height: 1.55;
		color: var(--text-secondary);
		text-wrap: pretty;
	}

	.credit {
		font-family: var(--font-mono);
		font-size: calc(11px * var(--step));
		letter-spacing: 0.08em;
		line-height: 1.8;
		color: var(--text-meta);
	}

	.dot {
		margin: 0 0.6em;
	}

	.credit a {
		color: var(--text-secondary);
		text-decoration: underline;
		text-underline-offset: 3px;
	}

	.credit a.read {
		color: var(--fox-text);
		text-decoration: none;
	}

	.credit a:hover {
		color: var(--fox-text);
	}

	.more {
		align-self: flex-start;
		min-height: var(--target);
		padding: 0 calc(30px * var(--step));
		border-radius: var(--radius-pill);
		border: 1px solid var(--edge-strong);
		background: none;
		color: var(--text);
		font-size: calc(14px * var(--step));
		font-weight: 600;
	}

	@media (prefers-reduced-motion: reduce) {
		.rest,
		.inner {
			transition-duration: 120ms;
		}
	}

	@media (max-width: 720px) {
		.bio {
			max-width: none;
			padding: 24px 20px;
		}

		.summary,
		.inner p {
			font-size: 16px;
		}
	}
</style>
