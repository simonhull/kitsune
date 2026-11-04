<script lang="ts">
	import { resolve } from '$app/paths';
	import Icon from './Icon.svelte';

	interface Props {
		route: 'home' | 'album';
		healthIssues: number | undefined;
	}

	let { route, healthIssues }: Props = $props();
</script>

<header class="bar">
	<nav aria-label="Main" class="nav" data-route={route}>
		<a class="fox" href={resolve('/')} aria-label="Kitsune home" data-sveltekit-reset="false"
			><Icon name="fox" size={22} /></a
		>
		<a
			class="item"
			href={resolve('/')}
			aria-current={route === 'home' ? 'page' : undefined}
			data-sveltekit-reset="false">Home</a
		>
		<span class="item" aria-disabled="true" aria-current={route === 'album' ? 'page' : undefined}
			>Albums</span
		>
		<span class="fold wide"><span class="item" aria-disabled="true">Artists</span></span>
		<span class="fold wide home-only">
			<span class="fold-in"><span class="item" aria-disabled="true">Playlists</span></span>
		</span>
		<span class="fold wide home-only">
			<span class="fold-in">
				<span class="item health" aria-disabled="true">
					Health
					{#if healthIssues !== undefined}<span class="badge">{healthIssues}</span>{/if}
				</span>
			</span>
		</span>
		<span class="search" aria-disabled="true" aria-label="Search">
			<Icon name="search" size={16} />
			<span class="fold home-only wide">
				<span class="fold-in">
					<span class="search-label">Search or jump to…<span class="key">⌘K</span></span>
				</span>
			</span>
		</span>
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
		transition:
			background var(--morph) var(--curve-morph),
			border-color var(--morph) var(--curve-morph),
			box-shadow var(--morph) var(--curve-morph);
	}

	.nav[data-route='album'] {
		background: color-mix(in srgb, var(--stage) 50%, transparent);
		border-color: rgb(255 255 255 / 10%);
		box-shadow: none;
	}

	.fox {
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

	[data-route='album'] .item {
		color: var(--wash);
	}

	a.item:hover {
		color: var(--fox);
	}

	.item[aria-current='page'] {
		padding: 0 18px;
		background: rgb(255 255 255 / 10%);
		color: var(--text);
		font-weight: 600;
	}

	[data-route='album'] .item[aria-current='page'] {
		background: rgb(255 255 255 / 14%);
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
		padding: 0 14px;
		border-radius: var(--radius-pill);
		background: rgb(255 255 255 / 6%);
		transition:
			background var(--morph) var(--curve-morph),
			margin var(--morph) var(--curve-morph);
		display: flex;
		align-items: center;
		color: var(--text-meta);
		font-size: 14px;
	}

	[data-route='album'] .search {
		margin-left: 0;
		background: rgb(255 255 255 / 8%);
		color: var(--wash);
	}

	.search-label {
		display: flex;
		align-items: center;
		gap: 12px;
		padding-left: 12px;
		margin-right: -6px;
	}

	.key {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		color: var(--text-secondary);
		background: rgb(255 255 255 / 8%);
		border-radius: var(--radius-pill);
		padding: 4px 10px;
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

	[data-route='album'] .home-only {
		grid-template-columns: 0fr;
		opacity: 0;
		margin-right: -4px;
	}

	[data-route='album'] .search .home-only {
		margin-right: 0;
	}

	@media (prefers-reduced-motion: reduce) {
		.nav,
		.nav * {
			transition-duration: 120ms;
		}
	}

	@media (max-width: 900px) {
		.wide {
			display: none;
		}
	}
</style>
