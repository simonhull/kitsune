<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { palettes } from '#lib/cover/cover.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import type { LastfmSwitches, Listening } from '#lib/library/source.ts';
	import {
		cardOf,
		countsLine,
		lastSentLine,
		playsWaiting,
		resultLine,
		resultOf,
		retryLine,
		roleLine,
		type ConnectResult
	} from '#lib/pages/you.ts';
	import { theme, themeChoices, type Theme, type ThemeChoice } from '#lib/theme/theme.svelte.ts';

	/** How often the words that depend on the clock are worked out again. */
	const tickMs = 15_000;
	/** How long the one-line result stays, fades included. */
	const resultMs = 4200;

	type SwitchKey = keyof Required<LastfmSwitches>;

	const switches: readonly { key: SwitchKey; name: string; hint: string }[] = [
		{
			key: 'scrobble',
			name: 'Scrobble what I play',
			hint: 'Every play that counts, sent from the server'
		},
		{
			key: 'nowPlaying',
			name: 'Show what’s playing now',
			hint: 'Your profile shows the song while it plays'
		},
		{
			key: 'loves',
			name: 'Send hearts as loves',
			hint: 'Hearting a song loves it; un-hearting un-loves it'
		}
	];

	const user = $derived(library.user);
	const status = $derived(library.lastfm);
	const card = $derived(status === null ? null : cardOf(status));
	let listening = $state.raw<Listening | null>(null);
	let now = $state(Date.now());
	let result = $state<ConnectResult | null>(null);
	/** Says what went wrong with the last thing asked for here, until the next. */
	let trouble = $state('');
	let busy = $state(false);

	onMount(() => {
		// Every visit asks: a play, a heart or Last.fm may have moved on since.
		void library.refreshLastfm();
		library.listening().then(
			(counts) => {
				listening = counts;
			},
			() => undefined
		);
		const arrived = resultOf(page.url.searchParams.get('lastfm'));
		let fade: ReturnType<typeof setTimeout> | undefined;
		if (arrived !== null) {
			result = arrived;
			// The line has said it; a reload mustn't say it again.
			replaceState(resolve('/you'), {});
			fade = setTimeout(() => {
				result = null;
			}, resultMs);
		}
		const clock = setInterval(() => {
			now = Date.now();
		}, tickMs);
		return () => {
			clearInterval(clock);
			clearTimeout(fade);
		};
	});

	async function connect(): Promise<void> {
		busy = true;
		trouble = '';
		try {
			const url = await library.connectLastfm();
			if (library.simulated) {
				// The fixtures have no last.fm to visit, and have already said yes.
				await library.refreshLastfm();
				result = 'connected';
				return;
			}
			location.assign(url);
		} catch {
			trouble = "Last.fm couldn't be reached from here just now. Try again in a moment.";
		} finally {
			busy = false;
		}
	}

	async function disconnect(): Promise<void> {
		busy = true;
		trouble = '';
		try {
			await library.disconnectLastfm();
		} catch {
			trouble = "That didn't disconnect. Try again.";
		} finally {
			busy = false;
		}
	}

	async function flip(key: SwitchKey, on: boolean): Promise<void> {
		trouble = '';
		try {
			await library.setLastfm({ [key]: on });
		} catch {
			trouble = "That switch couldn't be saved, so it's back as it was.";
		}
	}

	const themeNames: Record<ThemeChoice, string> = {
		system: 'System',
		light: 'Light',
		dark: 'Dark'
	};
	/** Each tile is a small page in its theme; System's is both, split down the middle. */
	const themeLayers: Record<ThemeChoice, readonly Theme[]> = {
		system: ['light', 'dark'],
		light: ['light'],
		dark: ['dark']
	};
	/** The previews' album: a generated cover's ground, so it is the same in every theme. */
	const sampleArt = palettes.at(0)?.base;
	const chosen = $derived(theme.choice ?? 'system');

	function pickTheme(choice: ThemeChoice): void {
		theme.choose(choice);
		library.prefs.put('ui.theme', choice);
		// One click is the whole change: nothing to wait for.
		void library.prefs.flush();
	}

	async function signOut(): Promise<void> {
		try {
			await library.signOut();
		} catch {
			trouble = "Signing out didn't work. Try again.";
		}
	}
	import ShuffleCard from '#lib/shuffle/ShuffleCard.svelte';
</script>

<svelte:head>
	<title>You · Kitsune</title>
</svelte:head>

{#if user !== null}
	<div class="you">
		<div class="who" data-morph-part="intro" data-morph-key="who">
			<p class="mono role">{roleLine(user.role)}</p>
			<h1>{user.name}</h1>
			<p class="mono counts" aria-live="polite">
				{listening === null ? ' ' : countsLine(listening)}
			</p>
			<div class="who-actions">
				{#if library.admin}
					<a class="ghost" href={resolve('/server')}
						>Server settings <span class="chevron" aria-hidden="true">›</span></a
					>
				{/if}
				<button type="button" class="ghost" onclick={() => void signOut()}>Sign out</button>
			</div>
		</div>

		<section aria-labelledby="looks" class="looks" data-morph-part="panel">
			<h2 id="looks" class="mono">HOW KITSUNE LOOKS</h2>
			<div class="appearance">
				<h3 id="theme">Theme</h3>
				<div class="tiles" role="radiogroup" aria-labelledby="theme">
					{#each themeChoices as choice (choice)}
						<label class="tile" class:on={chosen === choice}>
							<input
								type="radio"
								name="theme"
								value={choice}
								checked={chosen === choice}
								onchange={() => {
									pickTheme(choice);
								}}
							/>
							<span class="preview" aria-hidden="true">
								{#each themeLayers[choice] as layer, index (layer)}
									<span class="layer" class:split={index > 0} data-theme={layer}>
										<span class="art" style:background={sampleArt}></span>
										<span class="line"></span>
										<span class="line short"></span>
										<span class="bar"></span>
									</span>
								{/each}
							</span>
							<span class="choice"><span class="dot"></span>{themeNames[choice]}</span>
						</label>
					{/each}
				</div>
				<p class="note">
					System follows this device, and switches when it does. Right now that’s {theme.prefersLight
						? 'light'
						: 'dark'}.
				</p>
			</div>
		</section>

		<section aria-labelledby="shuffle-head" class="shuffles" data-morph-part="panel">
			<h2 id="shuffle-head" class="mono">HOW SHUFFLE CHOOSES</h2>
			<ShuffleCard />
		</section>

		<section aria-labelledby="where" class="where" data-morph-part="panel">
			<div class="where-head">
				<h2 id="where" class="mono">WHERE YOUR LISTENING GOES</h2>
				{#if result !== null}
					{#key result}
						<p class="result" class:bad={result !== 'connected'} role="status">
							{resultLine(result, status?.name)}
						</p>
					{/key}
				{/if}
			</div>

			{#if status !== null && card !== null}
				<div class="card" data-card={card}>
					<div class="head">
						<span class="service">Last.fm</span>
						{#if card === 'connected'}
							<span class="mono state on">● CONNECTED</span>
						{:else if card === 'waiting'}
							<span class="mono state">◌ WAITING</span>
						{:else if card === 'reconnect'}
							<span class="mono state on">NEEDS RECONNECTING</span>
						{/if}
					</div>

					{#if card === 'unset'}
						{#if library.admin}
							<p class="body lead">This server has no Last.fm key yet.</p>
							<p class="body meta">
								Add one and everyone here can send their listening to their own Last.fm account. It
								takes a minute: Last.fm gives the key and secret, you paste them in.
							</p>
							<div class="actions">
								<a class="solid" href={resolve('/server#lastfm')}>Add a key</a>
								<a
									class="quiet"
									href="https://www.last.fm/api/account/create"
									target="_blank"
									rel="noopener noreferrer">Get one from Last.fm ↗</a
								>
							</div>
						{:else}
							<p class="body lead">Not on this server yet.</p>
							<p class="body meta">
								Ask whoever runs it to add a Last.fm key, then connect your account here.
							</p>
						{/if}
					{:else if card === 'none'}
						<p class="body">
							Send what you play here to your Last.fm profile. You’ll go to last.fm to say yes, then
							come straight back.
						</p>
						<div class="actions">
							<button type="button" class="solid" disabled={busy} onclick={() => void connect()}
								>Connect Last.fm</button
							>
						</div>
					{:else if card === 'reconnect'}
						<p class="body">
							Last.fm stopped accepting Kitsune for <strong>{status.name}</strong>, probably because
							access was removed on last.fm.
							{#if status.waiting > 0}
								<strong>{playsWaiting(status.waiting)}</strong>
								{status.waiting === 1 ? 'is kept and goes' : 'are kept and go'} once you reconnect.
							{/if}
						</p>
						<div class="actions">
							<button type="button" class="solid" disabled={busy} onclick={() => void connect()}
								>Reconnect</button
							>
							<button type="button" class="ghost" disabled={busy} onclick={() => void disconnect()}
								>Disconnect</button
							>
						</div>
					{:else}
						{#if card === 'waiting'}
							<p class="body">
								Last.fm isn’t answering. <strong>{playsWaiting(status.waiting)}</strong>
								{status.waiting === 1
									? 'is waiting on the server and goes'
									: 'are waiting on the server and go'} in order when it’s back. Nothing to do.
							</p>
							<p class="mono retry">{retryLine(status.retryAt, now)}</p>
						{:else}
							<p class="as">
								as <strong>{status.name}</strong> · {lastSentLine(status.lastSentAt, now)}
							</p>
						{/if}
						<div class="switches">
							{#each switches as s (s.key)}
								<label class="switch">
									<span class="words">
										<span class="name">{s.name}</span>
										<span class="hint">{s.hint}</span>
									</span>
									<input
										type="checkbox"
										checked={status[s.key]}
										onchange={(event) => void flip(s.key, event.currentTarget.checked)}
									/>
									<span class="track" aria-hidden="true"><span class="knob"></span></span>
								</label>
							{/each}
						</div>
						<div class="actions">
							<button type="button" class="ghost" disabled={busy} onclick={() => void disconnect()}
								>Disconnect</button
							>
						</div>
					{/if}
				</div>
				{#if card !== 'unset'}
					<p class="footnote">
						Plays go when they count, the same rule as your play history. If Last.fm is down they
						wait on the server and go later, in order.
					</p>
				{/if}
			{/if}
			<p class="trouble" role="status">{trouble}</p>
		</section>
	</div>
{/if}

<style>
	.you {
		max-width: 1440px;
		margin: 0 auto;
		padding: 120px 64px 160px;
		display: grid;
		grid-template-columns: minmax(0, 560px) minmax(0, 656px);
		justify-content: space-between;
		align-items: start;
		gap: 32px;
	}

	.mono {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		font-weight: 400;
		letter-spacing: 0.08em;
	}

	.role,
	h2 {
		color: var(--text-meta);
	}

	h1 {
		margin-top: 18px;
		font-size: var(--type-hero);
		font-weight: 900;
		letter-spacing: var(--tracking-hero);
		line-height: 0.82;
		overflow-wrap: anywhere;
	}

	.counts {
		margin-top: 28px;
		min-height: 1lh;
		color: var(--text-secondary);
	}

	button {
		height: var(--target);
		border-radius: var(--radius-pill);
		font-size: 14px;
		transition:
			background var(--morph-fast) var(--curve-morph),
			border-color var(--morph-fast) var(--curve-morph),
			opacity var(--morph-fast) var(--curve-morph);
	}

	button:disabled {
		opacity: 0.6;
		cursor: default;
	}

	.ghost {
		padding: 0 18px;
		border: 1px solid var(--edge-strong);
		background: transparent;
		color: var(--text);
		font-weight: 600;
	}

	.ghost:hover:not(:disabled) {
		border-color: var(--edge-hover);
	}

	.solid {
		padding: 0 20px;
		border: 0;
		background: var(--fox);
		color: var(--fox-ink);
		font-weight: 700;
	}

	.who-actions {
		margin-top: 28px;
		display: flex;
		flex-wrap: wrap;
		gap: 10px;
	}

	a.ghost,
	a.solid,
	a.quiet {
		height: var(--target);
		display: inline-flex;
		align-items: center;
		gap: 10px;
		border-radius: var(--radius-pill);
		font-size: 14px;
		text-decoration: none;
	}

	a.solid,
	a.quiet {
		height: 48px;
	}

	a.solid {
		padding: 0 22px;
	}

	.quiet {
		padding: 0 16px;
		color: var(--text);
		font-weight: 600;
	}

	.quiet:hover {
		color: var(--fox-text);
	}

	.chevron {
		color: var(--text-meta);
	}

	.body.lead {
		color: var(--text);
		font-size: var(--type-body);
	}

	.where-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
		/* The line comes and goes without moving the card. */
		height: 16px;
	}

	/* Comes in, stays, and goes on its own: three steps forward, nothing played back. */
	.result {
		font-size: 13px;
		color: var(--fox-text);
		animation: said 4200ms var(--curve-morph) forwards;
	}

	.result.bad {
		color: var(--text-secondary);
	}

	@keyframes said {
		0% {
			opacity: 0;
			transform: translateY(4px);
		}

		7% {
			opacity: 1;
			transform: none;
		}

		82% {
			opacity: 1;
		}

		100% {
			opacity: 0;
		}
	}

	.card {
		margin-top: 16px;
		padding: 28px;
		border-radius: var(--radius-28);
		background: var(--card);
		border: 1px solid var(--edge-card);
	}

	.card[data-card='reconnect'] {
		border-color: var(--fox-line);
	}

	.card[data-card='unset'] {
		background: transparent;
		border: 1px dashed var(--edge-strong);
	}

	.head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
	}

	.service {
		font-size: var(--type-section);
		font-weight: 800;
		letter-spacing: var(--tracking-section);
	}

	[data-card='unset'] .service {
		color: var(--text-label);
	}

	.state {
		color: var(--text-secondary);
	}

	.state.on {
		color: var(--fox-text);
	}

	.as {
		margin-top: 6px;
		font-size: var(--type-body);
		color: var(--text-secondary);
	}

	.body {
		margin-top: 10px;
		font-size: 14px;
		line-height: 1.5;
		color: var(--text-secondary);
	}

	.body.meta {
		color: var(--text-meta);
	}

	strong {
		color: var(--text);
		font-weight: 600;
	}

	.retry {
		margin-top: 24px;
		color: var(--text-meta);
	}

	.switches {
		margin-top: 24px;
		display: flex;
		flex-direction: column;
	}

	.switch {
		position: relative;
		display: flex;
		align-items: center;
		gap: 16px;
		min-height: 56px;
		border-top: 1px solid var(--edge-card);
		cursor: pointer;
	}

	.words {
		flex: 1;
	}

	.name {
		display: block;
		font-size: var(--type-body);
		font-weight: 600;
	}

	.hint {
		display: block;
		margin-top: 2px;
		font-size: 13px;
		color: var(--text-meta);
	}

	/* A real checkbox, drawn as a switch: it keeps the keyboard and the label. */
	input {
		position: absolute;
		width: 1px;
		height: 1px;
		opacity: 0;
		margin: 0;
	}

	.track {
		position: relative;
		flex: none;
		width: 44px;
		height: 26px;
		border-radius: var(--radius-pill);
		background: var(--rail);
		transition: background var(--morph-fast) var(--curve-morph);
	}

	.knob {
		position: absolute;
		top: 3px;
		left: 3px;
		width: 20px;
		height: 20px;
		border-radius: 50%;
		background: var(--text-secondary);
		transition:
			transform var(--morph-fast) var(--curve-settle),
			background var(--morph-fast) var(--curve-morph);
	}

	input:checked + .track {
		background: var(--fox);
	}

	input:checked + .track .knob {
		transform: translateX(18px);
		background: var(--fox-ink);
	}

	input:focus-visible + .track {
		outline: 2px solid var(--fox-line);
		outline-offset: 2px;
	}

	.actions {
		margin-top: 20px;
		display: flex;
		gap: 10px;
	}

	[data-card='none'] .actions,
	[data-card='reconnect'] .actions {
		margin-top: 24px;
	}

	.footnote {
		margin: 16px 4px 0;
		font-size: 13px;
		line-height: 1.5;
		color: var(--text-meta);
	}

	.trouble {
		margin: 12px 4px 0;
		font-size: 13px;
		color: var(--problem);
	}

	.trouble:empty {
		display: none;
	}

	/* The Appearance board: how Kitsune looks heads the right column, and Last.fm follows it. */
	.who {
		grid-row: span 3;
	}

	.looks,
	.shuffles,
	.where {
		grid-column: 2;
	}

	.appearance {
		margin-top: 16px;
		padding: 32px;
		border-radius: var(--radius-28);
		background: var(--card);
		border: 1px solid var(--edge-card);
		display: flex;
		flex-direction: column;
		gap: 20px;
	}

	h3 {
		margin: 0;
		font-size: var(--type-section);
		font-weight: 800;
		letter-spacing: var(--tracking-section);
	}

	.tiles {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 16px;
	}

	.tile {
		position: relative;
		padding: 12px 12px 16px;
		border: 2px solid var(--edge-strong);
		border-radius: 22px;
		display: flex;
		flex-direction: column;
		gap: 12px;
		font-size: var(--type-body);
		font-weight: 600;
		cursor: pointer;
		transition: border-color var(--morph-fast) var(--curve-morph);
	}

	.tile:hover {
		border-color: var(--edge-hover);
	}

	.tile.on {
		border-color: var(--fox-line);
	}

	.tile:has(input:focus-visible) {
		outline: 2px solid var(--fox-line);
		outline-offset: 2px;
	}

	.preview {
		position: relative;
		display: block;
		height: 120px;
		border-radius: 14px;
		overflow: hidden;
		border: 1px solid var(--edge-raised);
	}

	/* A theme inside the page's: data-theme gives the layer that theme's tokens. */
	.layer {
		position: absolute;
		inset: 0;
		background: var(--ground);
	}

	.layer.split {
		clip-path: inset(0 0 0 50%);
	}

	.layer span {
		position: absolute;
	}

	.art {
		left: 14px;
		top: 16px;
		width: 44px;
		height: 44px;
		border-radius: 10px;
	}

	.line {
		left: 70px;
		right: 14px;
		top: 20px;
		height: 8px;
		border-radius: 4px;
		background: var(--text-secondary);
	}

	.line.short {
		top: 38px;
		right: calc(14px + (100% - 84px) / 3);
		opacity: 0.5;
	}

	.bar {
		left: 14px;
		right: 14px;
		bottom: 14px;
		height: 22px;
		border-radius: 8px;
		background: var(--fox);
	}

	.choice {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 0 4px;
	}

	.dot {
		flex: none;
		width: 18px;
		height: 18px;
		border-radius: 50%;
		border: 2px solid var(--edge-strong);
		transition:
			border-color var(--morph-fast) var(--curve-morph),
			background var(--morph-fast) var(--curve-morph);
	}

	.on .dot {
		border-color: var(--fox-line);
		background: var(--fox);
	}

	.note {
		font-size: 14px;
		line-height: 1.55;
		color: var(--text-meta);
	}

	@media (prefers-reduced-motion: reduce) {
		.result {
			animation-name: said-still;
		}

		@keyframes said-still {
			0%,
			82% {
				opacity: 1;
			}

			100% {
				opacity: 0;
			}
		}

		.track,
		.knob {
			transition-duration: 120ms;
		}
	}

	@media (max-width: 1100px) {
		.you {
			grid-template-columns: minmax(0, 1fr);
			padding: 106px 32px 160px;
			gap: 56px;
		}

		h1 {
			font-size: clamp(72px, 18vw, var(--type-hero));
		}

		.who,
		.looks,
		.shuffles,
		.where {
			grid-row: auto;
			grid-column: auto;
		}
	}

	@media (max-width: 720px) {
		.you {
			padding: 98px 16px 140px;
		}

		.appearance {
			padding: 20px;
		}

		.tiles {
			gap: 10px;
		}

		.tile {
			padding: 8px 8px 12px;
			border-radius: 18px;
		}

		.preview {
			height: 88px;
			border-radius: 10px;
		}

		.art {
			left: 10px;
			top: 10px;
			width: 30px;
			height: 30px;
			border-radius: 8px;
		}

		.line {
			left: 48px;
			right: 10px;
			top: 13px;
		}

		.line.short {
			top: 27px;
			right: calc(10px + (100% - 58px) / 3);
		}

		.bar {
			left: 10px;
			right: 10px;
			bottom: 10px;
			height: 16px;
			border-radius: 6px;
		}

		.choice {
			gap: 6px;
			padding: 0;
			font-size: 14px;
		}

		.dot {
			width: 14px;
			height: 14px;
		}
	}
</style>
