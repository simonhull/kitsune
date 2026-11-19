<script lang="ts">
	import { getContext } from 'svelte';
	import { resolve } from '$app/paths';
	import { settleTime } from '#lib/morph/choreography.ts';
	import { morphsKey } from '#lib/morph/engine.ts';
	import type { Morphs } from '#lib/morph/engines/spring.ts';
	import { motionOf } from '#lib/morph/journey.ts';
	import { morph } from '#lib/morph/registry.ts';
	import { flowingPill } from './flowingPill.ts';
	import Icon from './Icon.svelte';
	import type { NavItem, NavLook } from './routes.ts';
	import { navNote } from '#lib/organize/words.ts';
	import { navNote as gainNote } from '#lib/replaygain/words.ts';

	interface Props {
		look: NavLook;
		current: NavItem | null;
		healthIssues: number | undefined;
		/** On the You page, which the fox opens. */
		you: boolean;
		/** Last.fm needs reconnecting: plays are piling up. */
		attention: boolean;
		/** An organize run going, for its small note. */
		organizing?: { done: number; total: number } | null;
		/** A ReplayGain run going: its note takes the same slot, since the two never run at once. */
		writingGain?: { done: number; total: number } | null;
	}

	let {
		look,
		current,
		healthIssues,
		you,
		attention,
		organizing = null,
		writingGain = null
	}: Props = $props();

	const morphs = getContext<Morphs | undefined>(morphsKey);

	/**
	 * A room's own place pressed again: it goes nowhere. Its pill presses, and a
	 * room scrolled down is laid again at its top, the way it came in. In flight, the flight carries on.
	 */
	function pressed(event: MouseEvent): void {
		const link = event.target instanceof Element ? event.target.closest('a') : null;
		const elsewhere = event.metaKey || event.ctrlKey || event.shiftKey || event.altKey;
		if (
			link === null ||
			elsewhere ||
			event.button !== 0 ||
			link.pathname !== location.pathname ||
			// A query is a room's own view of itself, but Search's is what it holds: the field
			// pressed there goes to an empty Search, as it always has.
			(link.dataset['room'] === 'search' && link.search !== location.search)
		) {
			return;
		}
		event.preventDefault();
		if (link.getAttribute('aria-current') === 'page') {
			pressPill(link.closest('nav'));
		}
		if (scrollY > 0) {
			morphs?.relay(link);
		}
	}

	/** Hears every press of the nav's links, Enter included, which clicks a link too. */
	function presses(nav: HTMLElement) {
		nav.addEventListener('click', pressed);
		return {
			destroy() {
				nav.removeEventListener('click', pressed);
			}
		};
	}

	/**
	 * Out of Now playing the nav comes back where it stands: it takes its new shape at once, unseen,
	 * then fades in. Coming down from where it went would be its leaving played backward. The bar is
	 * marked in the same flush that changes its look, so the shape's own transitions never start.
	 */
	function comesBack(bar: HTMLElement, initial: NavLook) {
		let was = initial;
		let fallback = 0;
		function done(): void {
			clearTimeout(fallback);
			bar.removeAttribute('data-returning');
		}
		function faded(event: TransitionEvent): void {
			if (event.target === bar && event.propertyName === 'opacity') {
				done();
			}
		}
		bar.addEventListener('transitionend', faded);
		bar.addEventListener('transitioncancel', faded);
		return {
			update(next: NavLook) {
				const back = was === 'gone' && next !== 'gone' && next !== 'door';
				was = next;
				done();
				if (back) {
					bar.setAttribute('data-returning', '');
					// In case no transition runs at all: nothing may stay marked.
					fallback = window.setTimeout(done, 2000);
				}
			},
			destroy() {
				done();
				bar.removeEventListener('transitionend', faded);
				bar.removeEventListener('transitioncancel', faded);
			}
		};
	}

	/** The pill gives under the press and comes back, on the fast spring, wherever it is going. */
	function pressPill(nav: HTMLElement | null): void {
		const pill = nav?.querySelector<HTMLElement>('.pill');
		if (
			pill === null ||
			pill === undefined ||
			matchMedia('(prefers-reduced-motion: reduce)').matches
		) {
			return;
		}
		pill.animate(
			{ transform: ['scale(1)', 'scale(0.93)', 'scale(1)'], offset: [0, 0.35, 1] },
			{
				duration: settleTime(motionOf({ token: '--morph-fast', bounce: 0 })),
				easing: 'ease-out',
				// On top of where the pill is: a press never stops it flowing.
				composite: 'add'
			}
		);
	}
</script>

<header
	class="bar"
	data-route={look}
	inert={look === 'gone' || look === 'door'}
	use:comesBack={look}
>
	<nav aria-label="Main" class="nav" data-route={look} use:flowingPill={current} use:presses>
		<span class="pill" aria-hidden="true"></span>
		<a
			class="fox"
			href={resolve('/you')}
			data-room="you"
			aria-label={attention ? 'You: Last.fm needs reconnecting' : 'You'}
			aria-current={you ? 'page' : undefined}
			use:morph={{ id: 'fox', slot: 'nav', kind: 'control' }}
			><Icon name="fox" size={22} />{#if attention}<span class="dot" aria-hidden="true"
				></span>{/if}</a
		>
		<a
			class="item"
			href={resolve('/')}
			data-room="home"
			aria-current={current === 'home' ? 'page' : undefined}
			data-sveltekit-reset="false">Home</a
		>
		<a
			class="item"
			href={resolve('/library')}
			data-room="library"
			aria-current={current === 'library' ? 'page' : undefined}>Library</a
		>
		<a
			class="item"
			href={resolve('/albums')}
			data-room="albums"
			aria-current={current === 'albums' ? 'page' : undefined}>Albums</a
		>
		<a
			class="item"
			href={resolve('/artists')}
			data-room="artists"
			aria-current={current === 'artists' ? 'page' : undefined}>Artists</a
		>
		<a
			class="item playlists"
			href={resolve('/playlists')}
			data-room="playlists"
			aria-current={current === 'playlists' ? 'page' : undefined}>Playlists</a
		>
		{#if organizing !== null}
			<a
				class="organizing"
				href={resolve('/server/organize')}
				data-room="organize"
				aria-label={`Organizing: ${String(organizing.done)} of ${String(organizing.total)} songs`}
				>{navNote(organizing.done, organizing.total)}</a
			>
		{:else if writingGain !== null}
			<a
				class="organizing"
				href={resolve('/health')}
				aria-label={`Writing ReplayGain: ${String(writingGain.done)} of ${String(writingGain.total)} songs`}
				>{gainNote(writingGain.done, writingGain.total)}</a
			>
		{/if}
		<span class="fold wide home-only" inert={look === 'album'}>
			<span class="fold-in">
				<a class="item health" href={resolve('/health')} data-room="health">
					Health
					{#if healthIssues !== undefined && healthIssues > 0}<span class="badge"
							>{healthIssues.toLocaleString('en-US')}</span
						>{/if}
				</a>
			</span>
		</span>
		<a
			class="search"
			href={resolve('/search')}
			aria-label="Search"
			data-room="search"
			use:morph={{ id: 'field:search', slot: 'nav', kind: 'field' }}
		>
			<Icon name="search" size={16} />
			<span class="fold home-only wide">
				<span class="fold-in">
					<span class="search-label" use:morph={{ id: 'words:search', slot: 'nav', kind: 'title' }}
						>Search or jump to…</span
					>
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

	/*
	 * To the door the frame dissolves where it stands, not back and up: a picture of it does that
	 * under the door (lib/morph), and the bar itself is gone at once.
	 */
	.bar:global([data-morph-held]) {
		opacity: 0 !important;
		transition: none !important;
	}

	/*
	 * Back from Now playing it fades in where it stands, once the moment's top line has gone. Its
	 * place and its shape are set at once, while it is still unseen.
	 */
	.bar:global([data-returning]) {
		transition:
			opacity var(--morph) var(--curve-morph) 120ms,
			transform 0s,
			visibility 0s;
	}

	.bar:global([data-returning]) .nav,
	.bar:global([data-returning]) .nav * {
		transition-duration: 0s;
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
		border-color: var(--edge-raised);
		box-shadow: none;
	}

	/* One pill for the whole nav, so it can flow from item to item. */
	.pill {
		position: absolute;
		left: 0;
		top: 6px;
		height: var(--target);
		border-radius: var(--radius-pill);
		background: var(--fill);
		transition:
			background var(--morph) var(--curve-morph),
			opacity var(--morph-fast) var(--curve-morph);
	}

	:is([data-route='album'], [data-route='gone']) .pill {
		background: var(--pressed);
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
		/* The ring comes as the pill goes, and goes as it comes back. */
		transition: box-shadow var(--morph) var(--curve-morph);
	}

	/* Board A: on You the fox wears a ring of its own, since no item holds the pill. */
	.fox[aria-current='page'] {
		box-shadow:
			0 0 0 3px var(--ground),
			0 0 0 5px var(--fox);
	}

	/* Only for a Last.fm that needs reconnecting: the states board allows nothing louder. */
	.dot {
		position: absolute;
		top: -1px;
		right: -1px;
		width: 12px;
		height: 12px;
		border-radius: 50%;
		background: var(--fox);
		box-shadow: 0 0 0 3px var(--ground);
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
		color: var(--fox-text);
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
		background: var(--input);
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
		background: var(--hover);
		color: var(--wash);
	}

	a.search:hover {
		color: var(--fox-text);
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

		/* With the page's blend, which waits for nothing. */
		.bar:global([data-returning]) {
			transition-duration: 120ms, 0s, 0s;
			transition-delay: 0s;
		}

		.bar:global([data-returning]) .nav,
		.bar:global([data-returning]) .nav * {
			transition-duration: 0s;
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

	/*
	 * Everything left must still fit a phone: the targets stay 44px tall. Playlists goes the way
	 * Health does, and the You page, which the fox opens, links to them there.
	 */
	@media (max-width: 430px) {
		.playlists {
			display: none;
		}

		.item,
		.item[aria-current='page'] {
			padding: 0 7px;
		}

		.nav {
			gap: 2px;
		}

		.search {
			margin-left: 2px;
			padding: 0 14px;
		}
	}

	@media (max-width: 374px) {
		.item,
		.item[aria-current='page'] {
			padding: 0 3px;
			font-size: 13px;
		}
	}

	/* An organize run going: a small mono note, which opens its page. */
	.organizing {
		padding: 0 12px;
		font: 600 11px var(--font-mono);
		letter-spacing: 0.06em;
		color: var(--fox-text);
		text-decoration: none;
	}

	.organizing:hover {
		color: var(--text);
	}
</style>
