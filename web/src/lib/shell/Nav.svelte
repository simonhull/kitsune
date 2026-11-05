<script lang="ts">
	import { resolve } from '$app/paths';
	import { morph } from '#lib/morph/registry.ts';
	import { flowingPill } from './flowingPill.ts';
	import Icon from './Icon.svelte';
	import type { NavItem, NavLook } from './routes.ts';

	interface Props {
		look: NavLook;
		current: NavItem | null;
		healthIssues: number | undefined;
	}

	let { look, current, healthIssues }: Props = $props();
</script>

<header class="bar" data-route={look} inert={look === 'gone' || look === 'door'}>
	<nav aria-label="Main" class="nav" data-route={look} use:flowingPill={current}>
		<span class="pill" aria-hidden="true"></span>
		<a
			class="fox"
			href={resolve('/')}
			aria-label="Kitsune home"
			data-sveltekit-reset="false"
			use:morph={{ id: 'fox', slot: 'nav', kind: 'control' }}><Icon name="fox" size={22} /></a
		>
		<a
			class="item"
			href={resolve('/')}
			aria-current={current === 'home' ? 'page' : undefined}
			data-sveltekit-reset="false">Home</a
		>
		<span class="item" aria-disabled="true" aria-current={current === 'albums' ? 'page' : undefined}
			>Albums</span
		>
		<span class="item" aria-disabled="true">Artists</span>
		<span class="fold wide home-only">
			<span class="fold-in">
				<span class="item health" aria-disabled="true">
					Health
					{#if healthIssues !== undefined && healthIssues > 0}<span class="badge"
							>{healthIssues.toLocaleString('en-US')}</span
						>{/if}
				</span>
			</span>
		</span>
		<a class="search" href={resolve('/search')} aria-label="Search">
			<Icon name="search" size={16} />
			<span class="fold home-only wide">
				<span class="fold-in">
					<span class="search-label">Search or jump to…</span>
				</span>
			</span>
		</a>
	</nav>
</header>

<style>
	.bar {
		position: fixed;
		inset: 16px 0 auto;
		z-index: 3;
		display: flex;
		justify-content: center;
		padding: 0 16px;
		pointer-events: none;
	}

	/* On Now playing the nav goes away the way everything shallow does: back and up. */
	.bar {
		transition:
			opacity var(--morph-fast) var(--curve-morph),
			transform var(--morph) var(--curve-morph),
			visibility 0s;
	}

	.bar:is([data-route='gone'], [data-route='door']) {
		opacity: 0;
		visibility: hidden;
		transform: translateY(-20px) scale(0.94);
		transition:
			opacity var(--morph-fast) var(--curve-morph),
			transform var(--morph) var(--curve-morph),
			visibility 0s var(--morph-fast);
	}

	.nav {
		display: flex;
		align-items: center;
		gap: 4px;
		padding: 6px;
		border-radius: var(--radius-pill);
		background: var(--glass-fill);
		border: 1px solid var(--edge);
		backdrop-filter: var(--glass-blur);
		box-shadow: var(--glass-shadow);
		pointer-events: auto;
		white-space: nowrap;
		position: relative;
		transition:
			background var(--morph) var(--curve-morph),
			border-color var(--morph) var(--curve-morph),
			box-shadow var(--morph) var(--curve-morph);
	}

	.nav:is([data-route='album'], [data-route='gone']) {
		/* Heavier than the board's 50%: at that level the glow behind it fails AA on most albums. */
		background: color-mix(in srgb, var(--stage) 68%, transparent);
		border-color: rgb(255 255 255 / 10%);
		box-shadow: none;
	}

	/* One pill for the whole nav, so it can flow from item to item. */
	.pill {
		position: absolute;
		left: 0;
		top: 6px;
		height: var(--target);
		border-radius: var(--radius-pill);
		background: rgb(255 255 255 / 10%);
		transition:
			background var(--morph) var(--curve-morph),
			opacity var(--morph-fast) var(--curve-morph);
	}

	:is([data-route='album'], [data-route='gone']) .pill {
		background: rgb(255 255 255 / 14%);
	}

	.fox {
		position: relative;
		flex: none;
		width: var(--target);
		height: var(--target);
		border-radius: 50%;
		background: var(--fox);
		color: var(--fox-ink);
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.item {
		position: relative;
		height: var(--target);
		padding: 0 16px;
		border-radius: var(--radius-pill);
		display: flex;
		align-items: center;
		font-size: 14px;
		color: var(--text-secondary);
		transition:
			background var(--morph) var(--curve-morph),
			color var(--morph) var(--curve-morph);
	}

	:is([data-route='album'], [data-route='gone']) .item {
		color: var(--wash);
	}

	a.item:hover {
		color: var(--fox);
	}

	.item[aria-current='page'] {
		padding: 0 18px;
		color: var(--text);
		font-weight: 600;
	}

	:is([data-route='album'], [data-route='gone']) .item[aria-current='page'] {
		color: var(--text);
	}

	.health {
		padding: 0 12px 0 16px;
		gap: 8px;
	}

	.badge {
		min-width: 22px;
		height: 22px;
		padding: 0 6px;
		border-radius: var(--radius-pill);
		background: var(--fox);
		color: var(--fox-ink);
		font-size: 12px;
		font-weight: 700;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.search {
		height: var(--target);
		min-width: var(--target);
		margin-left: 4px;
		padding: 0 18px 0 14px;
		border-radius: var(--radius-pill);
		background: rgb(255 255 255 / 6%);
		transition:
			background var(--morph) var(--curve-morph),
			margin var(--morph) var(--curve-morph),
			padding var(--morph) var(--curve-morph);
		display: flex;
		align-items: center;
		color: var(--text-meta);
		font-size: 14px;
	}

	:is([data-route='album'], [data-route='gone']) .search {
		margin-left: 0;
		padding: 0 14px;
		background: rgb(255 255 255 / 8%);
		color: var(--wash);
	}

	a.search:hover {
		color: var(--fox);
	}

	.search-label {
		display: block;
		padding-left: 12px;
	}

	/* The nav really narrows on Album: its width animates, it isn't a scaled picture. */
	.fold {
		display: grid;
		grid-template-columns: 1fr;
		overflow: hidden;
		transition:
			grid-template-columns var(--morph) var(--curve-morph),
			opacity var(--morph-fast) var(--curve-morph),
			margin var(--morph) var(--curve-morph);
	}

	/* Padding lives inside, so a closed fold really reaches zero. */
	.fold-in {
		min-width: 0;
		overflow: hidden;
	}

	:is([data-route='album'], [data-route='gone']) .home-only {
		grid-template-columns: 0fr;
		opacity: 0;
		margin-right: -4px;
	}

	:is([data-route='album'], [data-route='gone']) .search .home-only {
		margin-right: 0;
	}

	@media (prefers-reduced-motion: reduce) {
		.bar,
		.nav,
		.nav * {
			transition-duration: 120ms;
		}
	}

	@media (max-width: 900px) {
		.wide {
			display: none;
		}

		.item,
		.item[aria-current='page'] {
			padding: 0 12px;
		}

		.search {
			padding: 0 14px;
		}
	}

	/* Everything left must still fit a small phone: the targets stay 44px tall. */
	@media (max-width: 360px) {
		.item,
		.item[aria-current='page'] {
			padding: 0 7px;
		}
	}
</style>
