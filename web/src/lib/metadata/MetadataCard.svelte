<!--
	Health's Metadata card (plan 072, Phase 2): artist art and bios from the internet. On a new server
	lookups are off, and the card offers them with exactly what they send; Turn on reshapes it in place
	into the pass's head. Then it says how far the pass has got, each source's state in words (a key's
	trouble leading to Server › Settings), the artists only Matching can give art, and the Credits.
	Administrators only. Progress is no event: the card reads again while a pass goes and it's on screen.
-->
<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { MediaQuery } from 'svelte/reactivity';
	import { resolve } from '$app/paths';
	import { library } from '#lib/library/library.svelte.ts';
	import { reshapeOn } from '#lib/morph/reshapeOn.svelte.ts';
	import { Surface } from '#lib/morph/surface.ts';
	import type { MetadataStatus } from './types.ts';
	import {
		chipLeadsToKey,
		chipWords,
		eyebrowWords,
		failureWords,
		figureWords,
		keysWords,
		never,
		noMbidWords,
		nowWords,
		offerTitle,
		passShare,
		restWords,
		sends,
		sourceName,
		sourceWhat,
		toWords
	} from './words.ts';

	/** How often a pass's progress is read while the card is on screen. */
	const pollMs = 2500;
	/** How long after lookups are turned on the card reads as if a pass went: one starts a moment later. */
	const startingMs = 8000;
	/** How often the times are worked out again. */
	const tickMs = 30_000;

	const phone = new MediaQuery('(max-width: 720px)', false);

	let status = $state.raw<MetadataStatus | null>(null);
	let shown = $state(false);
	let starting = $state(false);
	let turning = $state(false);
	let problem = $state('');
	let now = $state(Date.now());

	/** The card at rest on its page, reshaped in place when Turn on gives it its next face. */
	const surface = new Surface();
	reshapeOn(
		surface,
		() => 'card',
		() => (status === null ? '' : status.remote ? 'on' : 'offer')
	);

	let asking: Promise<void> | null = null;

	/** Reads the status, one ask at a time. A failure leaves what was known; a 503 draws no card. */
	function read(): Promise<void> {
		asking ??= library.metadata
			.status()
			.then((next) => {
				status = next;
				if (next.pass.running || !next.remote) {
					starting = false;
				}
			})
			.catch(() => undefined)
			.finally(() => {
				asking = null;
			});
		return asking;
	}

	onMount(() => {
		void read();
		const clock = setInterval(() => {
			now = Date.now();
		}, tickMs);
		return () => {
			clearInterval(clock);
		};
	});

	// An artist's art arriving, or a setting changed elsewhere, moves the counts and the sources.
	$effect(() => {
		void library.artistsHeard;
		void library.settingsHeard;
		untrack(() => {
			if (status !== null) {
				void read();
			}
		});
	});

	// While a pass goes and the card is on screen, its progress is read again, gently.
	$effect(() => {
		if (!shown || !(starting || status?.pass.running === true)) {
			return;
		}
		const poll = setInterval(() => void read(), pollMs);
		return () => {
			clearInterval(poll);
		};
	});

	/** Whether the card is on screen: nothing is polled for a card scrolled away. */
	function watched(element: HTMLElement): () => void {
		const seen = new IntersectionObserver(([entry]) => {
			shown = entry?.isIntersecting ?? false;
		});
		seen.observe(element);
		return () => {
			seen.disconnect();
		};
	}

	let startTimer: ReturnType<typeof setTimeout> | undefined;

	async function turnOn(): Promise<void> {
		problem = '';
		turning = true;
		try {
			await library.changeSettings({ 'metadata.remote': true });
			starting = true;
			clearTimeout(startTimer);
			startTimer = setTimeout(() => {
				starting = false;
			}, startingMs);
			await read();
		} catch {
			problem = 'Lookups weren’t turned on: the server couldn’t be reached. Try again.';
		} finally {
			turning = false;
		}
	}

	$effect(() => () => {
		clearTimeout(startTimer);
	});

	const fanart = $derived(status?.sources.find((source) => source.name === 'fanarttv'));
	const figure = $derived(status === null ? null : figureWords(status));
	const running = $derived(status?.pass.running === true);
	const eyebrow = $derived.by(() => {
		if (status === null) {
			return '';
		}
		return starting && status.remote && !running
			? 'ARTIST ART AND BIOS · STARTING'
			: eyebrowWords(status);
	});
</script>

{#if library.admin && status !== null}
	<section
		class="card"
		class:quiet={status.remote && !running}
		class:phone={phone.current}
		aria-labelledby="metadata-card"
		data-testid="metadata"
		{@attach surface.rests}
		{@attach watched}
	>
		<div class="top">
			<span class="eyebrow" class:hot={running || starting} id="metadata-card">{eyebrow}</span>
			{#if !phone.current}<span class="small">ONLY ADMINS</span>{/if}
		</div>

		{#if !status.remote}
			<h2 class="title">{offerTitle(status.artists.withMbid)}</h2>
			<p class="body">
				Kitsune knows them by their MusicBrainz IDs. Turned on, it asks for their pictures, logos
				and bios in the background. Nothing in your files changes.
			</p>
			<dl class="block">
				<div class="row">
					<dt>SENDS</dt>
					<dd>{sends}</dd>
				</div>
				<div class="row">
					<dt>NEVER</dt>
					<dd>{never}</dd>
				</div>
				<div class="row">
					<dt>TO</dt>
					<dd>{toWords(phone.current)}</dd>
				</div>
				<div class="row">
					<dt>KEYS</dt>
					<dd><a href={resolve('/server#art')}>{keysWords(fanart, phone.current)} ›</a></dd>
				</div>
			</dl>
			<div class="go">
				<button type="button" class="fox" disabled={turning} onclick={() => void turnOn()}
					>{turning ? 'Turning on…' : 'Turn on lookups'}</button
				>
				<span class="small">OFF ON A NEW SERVER · TURN OFF ANY TIME IN SETTINGS</span>
			</div>
			{#if problem !== ''}<p class="problem" role="alert">{problem}</p>{/if}
		{:else}
			{#if figure !== null}
				<div class="figure">
					<span class="number">{figure.figure}</span>
					<span class="of">{figure.of}</span>
				</div>
			{/if}
			<div
				class="bar"
				class:full={!running}
				role="progressbar"
				aria-label="Artists asked about in this pass"
				aria-valuemin={0}
				aria-valuemax={running ? status.pass.total : 1}
				aria-valuenow={running ? status.pass.done : 1}
			>
				<span style:width={`${String(running ? passShare(status.pass) * 100 : 100)}%`}></span>
			</div>
			<span class="small line" role="status"
				>{running
					? nowWords(status.pass, phone.current)
					: restWords(status.artists, status.nextRefresh, phone.current)}</span
			>

			<span class="label">SOURCES</span>
			<ul class="sources">
				{#each status.sources as source (source.name)}
					{@const failure = failureWords(source, now)}
					<li class="source" data-source={source.name}>
						<span class="words">
							<span class="name">{sourceName(source.name)}</span>
							<span class="what">{sourceWhat(source)}</span>
							{#if failure !== null}<span class="failure">{failure}</span>{/if}
						</span>
						{#if chipLeadsToKey(source)}
							<a
								class="chip"
								class:key={source.state === 'needsKey'}
								class:trouble={source.state === 'refused'}
								href={resolve('/server#art')}>{chipWords(source)} ›</a
							>
						{:else}
							<span
								class="chip"
								class:trouble={source.state === 'resting'}
								class:off={source.state === 'off'}>{chipWords(source)}</span
							>
						{/if}
					</li>
				{/each}
			</ul>

			{#if status.artists.withoutMbid > 0}
				<a class="mbid" href={resolve('/health/matching')}
					>{noMbidWords(status.artists.withoutMbid, phone.current)} ›</a
				>
			{/if}
			<span class="small sends"
				>{phone.current ? `SENDS ${sends}` : `SENDS ${sends} · NO ${never}`}</span
			>

			<div class="credits">
				<span class="label">CREDITS</span>
				<p>
					Artist art from <a href="https://fanart.tv" target="_blank" rel="noopener noreferrer"
						>fanart.tv</a
					>. Bios from
					<a href="https://www.wikipedia.org" target="_blank" rel="noopener noreferrer">Wikipedia</a
					>
					(<a
						href="https://creativecommons.org/licenses/by-sa/4.0/"
						target="_blank"
						rel="noopener noreferrer">CC BY-SA 4.0</a
					>). Portraits from
					<a href="https://commons.wikimedia.org" target="_blank" rel="noopener noreferrer"
						>Wikimedia Commons</a
					>, each under its own license. Who is who from
					<a href="https://musicbrainz.org" target="_blank" rel="noopener noreferrer">MusicBrainz</a
					>.
				</p>
			</div>
		{/if}
	</section>
{/if}

<style>
	.card {
		box-sizing: border-box;
		padding: calc(28px * var(--step));
		display: flex;
		flex-direction: column;
		border-radius: var(--radius-28);
		background: var(--card);
		border: 1px solid var(--fox-line);
	}

	.card.quiet {
		border-color: var(--edge-card);
	}

	.top {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 16px;
	}

	.eyebrow {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.eyebrow.hot {
		color: var(--fox-text);
	}

	.small,
	.label {
		font-family: var(--font-mono);
		font-size: calc(11px * var(--step));
		letter-spacing: 0.06em;
		color: var(--text-meta);
	}

	.label {
		letter-spacing: 0.1em;
	}

	a {
		color: var(--fox-text);
	}

	/* The offer. */
	.title {
		margin-top: calc(16px * var(--step));
		font-size: calc(32px * var(--step));
		font-weight: 800;
		letter-spacing: -0.035em;
		line-height: 1.1;
	}

	.body {
		margin-top: calc(12px * var(--step));
		max-width: 62ch;
		font-size: calc(15px * var(--step));
		line-height: 1.55;
		color: var(--text-secondary);
	}

	.block {
		margin: calc(20px * var(--step)) 0 0;
		padding: calc(12px * var(--step)) calc(16px * var(--step));
		display: flex;
		flex-direction: column;
		gap: calc(8px * var(--step));
		border-radius: var(--radius-16);
		background: var(--input);
		border: 1px solid var(--edge-card);
	}

	.row {
		display: grid;
		grid-template-columns: calc(70px * var(--step)) minmax(0, 1fr);
		gap: 12px;
		align-items: baseline;
		font-family: var(--font-mono);
		font-size: calc(11.5px * var(--step));
		letter-spacing: 0.06em;
		line-height: 1.5;
	}

	dt {
		color: var(--text-label);
	}

	dd {
		margin: 0;
		color: var(--text-secondary);
	}

	dd a {
		text-decoration: none;
	}

	.go {
		margin-top: calc(24px * var(--step));
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: calc(16px * var(--step));
	}

	.fox {
		height: calc(48px * var(--step));
		padding: 0 calc(26px * var(--step));
		border: 0;
		border-radius: var(--radius-pill);
		background: var(--fox);
		color: var(--fox-ink);
		font: 700 calc(15px * var(--step)) var(--font-sans);
		cursor: pointer;
	}

	.fox:disabled {
		opacity: 0.6;
		cursor: progress;
	}

	.fox:focus-visible {
		outline: 2px solid var(--fox-line);
		outline-offset: 3px;
	}

	.problem {
		margin-top: 12px;
		font-size: calc(14px * var(--step));
		color: var(--danger);
	}

	/* Lookups on: the pass, its sources, the artists Matching can help, and the Credits. */
	.figure {
		margin-top: calc(18px * var(--step));
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 4px calc(14px * var(--step));
	}

	.number {
		font-size: calc(64px * var(--step));
		font-weight: 900;
		letter-spacing: -0.05em;
		line-height: 0.9;
	}

	.of {
		font-size: calc(16px * var(--step));
		font-weight: 600;
		color: var(--text-secondary);
	}

	.bar {
		margin-top: calc(14px * var(--step));
		height: 8px;
		border-radius: 4px;
		background: var(--control-wash);
		overflow: hidden;
	}

	.bar span {
		display: block;
		height: 100%;
		background: var(--fox);
		transition: width 400ms var(--curve-settle);
	}

	.bar.full span {
		background: var(--edge-strong);
	}

	.line {
		margin-top: calc(12px * var(--step));
		line-height: 1.5;
	}

	.card > .label {
		margin-top: calc(24px * var(--step));
	}

	.sources {
		margin: calc(8px * var(--step)) 0 0;
		padding: 0;
		list-style: none;
		display: flex;
		flex-direction: column;
	}

	.source {
		display: flex;
		align-items: center;
		gap: 12px;
		min-height: calc(46px * var(--step));
		padding: calc(8px * var(--step)) 0;
		border-top: 1px solid var(--edge-card);
	}

	.words {
		flex: 1;
		min-width: 0;
		display: grid;
		grid-template-columns: calc(230px * var(--step)) minmax(0, 1fr);
		align-items: baseline;
		gap: 2px 12px;
	}

	.name {
		font-size: calc(14.5px * var(--step));
		font-weight: 600;
	}

	.what {
		font-size: calc(12.5px * var(--step));
		color: var(--text-meta);
	}

	.failure {
		grid-column: 1 / -1;
		font-family: var(--font-mono);
		font-size: calc(11px * var(--step));
		letter-spacing: 0.04em;
		color: var(--danger);
	}

	.chip {
		flex: none;
		padding: 0 calc(10px * var(--step));
		height: calc(24px * var(--step));
		display: inline-flex;
		align-items: center;
		border-radius: var(--radius-pill);
		border: 1px solid var(--edge);
		font: 500 calc(11px * var(--step)) var(--font-mono);
		letter-spacing: 0.06em;
		color: var(--text-secondary);
		white-space: nowrap;
		text-decoration: none;
	}

	.chip.key {
		border-color: var(--fox-line);
		color: var(--fox-text);
	}

	.chip.trouble {
		border-color: var(--danger-edge);
		color: var(--danger);
	}

	.chip.off {
		color: var(--text-label);
	}

	.mbid {
		margin-top: calc(14px * var(--step));
		font-family: var(--font-mono);
		font-size: calc(11px * var(--step));
		letter-spacing: 0.08em;
		line-height: 1.5;
		text-decoration: none;
	}

	.mbid:hover,
	.chip:is(a):hover,
	dd a:hover {
		text-decoration: underline;
	}

	.sends {
		margin-top: calc(12px * var(--step));
		color: var(--text-label);
	}

	.credits {
		margin-top: calc(20px * var(--step));
		padding-top: calc(16px * var(--step));
		border-top: 1px solid var(--edge);
		display: flex;
		flex-direction: column;
		gap: calc(10px * var(--step));
	}

	.credits p {
		font-size: calc(13.5px * var(--step));
		line-height: 1.55;
		color: var(--text-secondary);
	}

	/* A phone: each source's note under its name, and the offer's button the card's width. */
	.phone {
		padding: 20px;
		border-radius: var(--radius-24);
	}

	.phone .title {
		font-size: 26px;
	}

	.phone .words {
		display: flex;
		flex-direction: column;
	}

	.phone .go .fox {
		width: 100%;
	}

	@media (prefers-reduced-motion: reduce) {
		.bar span {
			transition-duration: 120ms;
		}
	}
</style>
