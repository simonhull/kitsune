<script lang="ts">
	import { onDestroy, tick } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { library } from '#lib/library/library.svelte.ts';
	import { SetupError } from '#lib/library/source.ts';
	import { morph } from '#lib/morph/registry.ts';
	import {
		checkPasswords,
		fieldOf,
		problemLine,
		setupLine,
		type SetupProblem
	} from '#lib/pages/setup.ts';
	import Icon from '#lib/shell/Icon.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	// What the server said on the way in. A server that didn't answer is taken as open, from home:
	// making the account says otherwise, and the page follows.
	const status = $derived(data.status ?? { needed: true, codeRequired: false });
	// Each holds what the server said until the page learns better, then what it learned.
	let done = $derived(!status.needed);
	let codeRequired = $derived(status.codeRequired);

	let busy = $state(false);
	let problem = $state.raw<SetupProblem | null>(null);
	/** Seconds until the server will listen again, while it's refusing. */
	let waiting = $state(0);
	let countdown: ReturnType<typeof setInterval> | undefined;

	let nameField: HTMLInputElement | undefined = $state();
	let passwordField: HTMLInputElement | undefined = $state();
	let againField: HTMLInputElement | undefined = $state();
	let codeField: HTMLInputElement | undefined = $state();

	const message = $derived(problemLine(problem, waiting));
	const wrong = $derived(fieldOf(problem));

	/** Counts down and clears itself, as sign-in does: nobody should have to reload. */
	function holdOff(seconds: number): void {
		const until = Date.now() + seconds * 1000;
		waiting = seconds;
		clearInterval(countdown);
		countdown = setInterval(() => {
			waiting = Math.ceil((until - Date.now()) / 1000);
			if (waiting <= 0) {
				clearInterval(countdown);
				waiting = 0;
				problem = null;
			}
		}, 1000);
	}

	/**
	 * The name stays, because it's usually right. A wrong code keeps the passwords, since nothing is
	 * wrong with them, and the code takes the focus; anything else clears both passwords and the
	 * first takes it, or the name when it's the name.
	 */
	async function refused(next: SetupProblem): Promise<void> {
		busy = false;
		if (next.kind === 'done') {
			done = true;
			return;
		}
		problem = next;
		if (next.kind === 'throttled') {
			holdOff(next.retryAfterSeconds);
		}
		if (next.kind === 'code') {
			codeRequired = true;
			await tick();
			codeField?.focus();
			codeField?.select();
			return;
		}
		for (const field of [passwordField, againField]) {
			if (field !== undefined) {
				field.value = '';
			}
		}
		(next.kind === 'name' ? nameField : passwordField)?.focus();
	}

	async function submit(event: SubmitEvent & { currentTarget: HTMLFormElement }): Promise<void> {
		event.preventDefault();
		if (busy || waiting > 0) {
			return;
		}
		const fields = new FormData(event.currentTarget);
		const name = String(fields.get('name'));
		const password = String(fields.get('password'));
		const own = checkPasswords(password, String(fields.get('again')));
		if (own !== null) {
			await refused(own);
			return;
		}
		busy = true;
		problem = null;
		try {
			await library.setup(name, password, codeRequired ? String(fields.get('code')) : undefined);
		} catch (error) {
			await refused(error instanceof SetupError ? error.failure : { kind: 'unavailable' });
			return;
		}
		// Home needs its units on the page before the fox leaves, or it has nothing to arrive.
		await library.loadShell();
		// Replacing, so Back never returns to the door.
		await goto(resolve('/'), { replaceState: true });
	}

	/** Puts the caret in the first field, where a keyboard won't jump up uninvited. */
	function focusOnDesktop(element: HTMLInputElement): void {
		if (matchMedia('(hover: hover)').matches) {
			element.focus();
		}
	}

	onDestroy(() => {
		clearInterval(countdown);
	});
</script>

<svelte:head>
	<title>{done ? 'Already set up' : 'Make the first account'} · Kitsune</title>
</svelte:head>

<div class="sleeve">
	<p class="line" data-morph-part="intro">{setupLine(done)}</p>
	<div class="words" data-morph-part="words">
		{#if done}
			<h1>Already<br />set up<span class="period">.</span></h1>
			<p class="lede">Kitsune has an account now, so this page is closed.</p>
		{:else}
			<h1>It's<br />yours<span class="period">.</span></h1>
			<p class="lede">
				Make the first account. It runs this server: it scans the library, edits tags and adds
				everyone else. Once it exists, this page is gone for good.
			</p>
		{/if}
	</div>

	{#if done}
		<div class="card" data-morph-part="panel">
			<a class="button" href={resolve('/login')}>Go to sign in</a>
			<p class="hint">If that wasn’t you, ask whoever runs the server.</p>
		</div>
	{:else}
		<form class="card" data-morph-part="panel" onsubmit={submit}>
			<label>
				<span>Your name</span>
				<input
					name="name"
					autocomplete="username"
					autocapitalize="none"
					autocorrect="off"
					spellcheck="false"
					required
					aria-invalid={wrong === 'name'}
					bind:this={nameField}
					use:focusOnDesktop
				/>
			</label>
			<label>
				<span>Password</span>
				<input
					name="password"
					type="password"
					autocomplete="new-password"
					required
					aria-invalid={wrong === 'password'}
					aria-describedby="setup-hint"
					bind:this={passwordField}
				/>
			</label>
			<label>
				<span>Password again</span>
				<input
					name="again"
					type="password"
					autocomplete="new-password"
					required
					bind:this={againField}
				/>
			</label>
			{#if codeRequired}
				<label>
					<span>Setup code</span>
					<input
						name="code"
						autocomplete="one-time-code"
						autocapitalize="none"
						autocorrect="off"
						spellcheck="false"
						required
						aria-invalid={wrong === 'code'}
						aria-describedby="code-hint"
						bind:this={codeField}
					/>
				</label>
				<p class="hint" id="code-hint">
					It's in the server's log, printed at startup: <code>docker compose logs kitsune</code>
				</p>
			{/if}
			<button type="submit" disabled={busy || waiting > 0} aria-busy={busy}>
				{busy ? 'Making it yours…' : 'Make it mine'}
			</button>
			<p class="problem" role="alert">{message}</p>
			<p class="hint" id="setup-hint" hidden={message !== ''}>
				At least 8 characters. You can add people later from the server.
			</p>
		</form>
	{/if}

	<span class="fox" use:morph={{ id: 'fox', slot: 'door', kind: 'control' }}
		><Icon name="fox" size={26} /></span
	>
</div>

<style>
	/*
	 * Board B, "It's yours": at 1440 × 900 every measure is the board's. The headline and its spacing
	 * scale with the width from there, and below 900 px the card stacks under the words.
	 */
	.sleeve {
		--hero: clamp(72px, 11.67vw, 168px);
		position: relative;
		min-height: 100dvh;
		display: grid;
		grid-template-columns: minmax(0, 1fr) 400px;
		grid-template-rows: auto auto 1fr;
		column-gap: 48px;
		/* The board's 140 is from the window's edge: the scrollbar gutter the app keeps comes out of it. */
		padding: 64px max(48px, calc(clamp(64px, 9.75vw, 140px) - (100vw - 100%))) 64px 64px;
	}

	.line {
		grid-area: 1 / 1;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		line-height: 16px;
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.words {
		grid-area: 2 / 1;
	}

	h1 {
		/* The board sets the glyphs 8 px left of the column, where their side bearing lines them up. */
		margin: calc(var(--hero) * 0.4167) 0 0 -8px;
		font-size: var(--hero);
		font-weight: 900;
		letter-spacing: var(--tracking-hero);
		line-height: 0.82;
	}

	.period {
		color: var(--fox-text);
	}

	.lede {
		max-width: 520px;
		margin-top: calc(var(--hero) * 0.5625);
		font-size: var(--type-title);
		line-height: 1.55;
		color: var(--text-secondary);
	}

	/* The nav's mark, small: the one thing that goes with you into the app. */
	.fox {
		position: absolute;
		left: 64px;
		bottom: 64px;
		width: 26px;
		height: 26px;
		border-radius: 50%;
		background: var(--fox);
		color: var(--fox-ink);
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.card {
		grid-area: 1 / 2 / -1 / 3;
		align-self: center;
		padding: 36px;
		border-radius: var(--radius-28);
		background: var(--card);
		border: 1px solid var(--edge-card);
		display: flex;
		flex-direction: column;
		gap: 16px;
	}

	label {
		display: flex;
		flex-direction: column;
		gap: 8px;
		font-size: 14px;
		font-weight: 600;
		color: var(--text-secondary);
	}

	input {
		height: 52px;
		padding: 0 20px;
		border: 1px solid var(--edge-strong);
		border-radius: var(--radius-pill);
		background: var(--input);
		color: var(--text);
		caret-color: var(--fox-text);
		/* Under 16 px, iOS zooms the page on focus. */
		font: 400 16px var(--font-sans);
		transition: border-color var(--morph-fast) var(--curve-morph);
	}

	input:hover {
		border-color: var(--edge-hover);
	}

	/* The field a refusal is about, as the states board marks it. */
	input[aria-invalid='true'] {
		border-color: var(--fox-text);
	}

	/* The browser's yellow has no place here. */
	input:-webkit-autofill {
		-webkit-text-fill-color: var(--text);
		box-shadow: inset 0 0 0 52px var(--input-solid);
		caret-color: var(--fox-text);
	}

	button,
	.button {
		height: 52px;
		margin-top: 8px;
		border: 0;
		border-radius: var(--radius-pill);
		background: var(--fox);
		color: var(--fox-ink);
		font: 700 16px var(--font-sans);
		display: flex;
		align-items: center;
		justify-content: center;
		text-decoration: none;
		transition: background var(--morph-fast) var(--curve-morph);
	}

	.button {
		margin-top: 0;
	}

	button:disabled {
		background: color-mix(in srgb, var(--fox) 45%, var(--ground));
		cursor: default;
	}

	.hint {
		font-size: 13px;
		line-height: 1.5;
		color: var(--text-label);
	}

	/* A command copied by hand reads best unbroken. */
	code {
		font-family: var(--font-mono);
		color: var(--text-secondary);
		white-space: nowrap;
	}

	.problem {
		font-size: 14px;
		line-height: 1.45;
		color: var(--fox-text);
	}

	.problem:empty {
		display: none;
	}

	@media (max-width: 900px) {
		.sleeve {
			--hero: clamp(64px, 18vw, 120px);
			grid-template-columns: minmax(0, 1fr);
			grid-template-rows: auto;
			row-gap: 40px;
			padding: 40px 20px 112px;
		}

		.words {
			grid-area: auto;
		}

		h1 {
			margin-top: 0;
			margin-left: -4px;
		}

		.lede {
			margin-top: 24px;
		}

		.card {
			grid-area: auto;
			width: 100%;
			max-width: 440px;
			align-self: start;
			padding: 28px;
		}

		.fox {
			left: 20px;
			bottom: 40px;
		}
	}
</style>
