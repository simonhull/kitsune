<script lang="ts">
	import { onDestroy, onMount, tick, untrack } from 'svelte';
	import { fly } from 'svelte/transition';
	import { library } from '#lib/library/library.svelte.ts';
	import { ResetError, type ResetFailure } from '#lib/library/source.ts';
	import {
		findCodeCommand,
		formatCode,
		resetFieldOf,
		resetProblemLine,
		resetRefusal,
		ranOut,
		timeLeftLine,
		type ResetProblem
	} from './passwords.ts';
	import { checkPasswords } from './setup.ts';

	/**
	 * The two steps of a forgotten password, as a card where the sign-in form was: a name, then the
	 * code from the server's log and a new password. Every step comes in forwards, Back included.
	 */
	interface Props {
		/** The name typed at sign-in, carried over. */
		name: string;
		/** Back to sign in, with the name as it now stands. */
		onback: (name: string) => void;
		/** Signed in with the new password: the page takes it from here. */
		ondone: () => Promise<void>;
	}

	let { name: initialName, onback, ondone }: Props = $props();

	type Step = 'name' | 'sent' | 'code' | 'done';

	// The sign-in form's name is only where this starts.
	let name = $state(untrack(() => initialName));
	let step = $state<Step>('name');
	let busy = $state(false);
	let problem = $state.raw<ResetProblem | null>(null);
	/** When this device asked for the code it's waiting on, by its own clock. */
	let askedAt = $state(0);
	let now = $state(Date.now());
	/** Seconds until the server will listen again, while it's refusing. */
	let waiting = $state(0);
	let copied = $state(false);
	let code = $state('');

	let nameField: HTMLInputElement | undefined = $state();
	let codeField: HTMLInputElement | undefined = $state();
	let passwordField: HTMLInputElement | undefined = $state();
	let againField: HTMLInputElement | undefined = $state();

	/** Each step still arrives, without moving, for anyone who asked for less motion. */
	const still =
		typeof matchMedia !== 'undefined' && matchMedia('(prefers-reduced-motion: reduce)').matches;

	let countdown: ReturnType<typeof setInterval> | undefined;
	let clock: ReturnType<typeof setInterval> | undefined;

	const message = $derived(resetProblemLine(problem, waiting, step !== 'code'));
	const wrong = $derived(resetFieldOf(problem));
	const expired = $derived(
		step === 'code' && (problem?.kind === 'expired' || ranOut(askedAt, now))
	);

	/** Counts down and clears itself: nobody should have to reload to be let back in. */
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

	function failed(error: unknown): ResetFailure {
		return error instanceof ResetError ? error.failure : { kind: 'unavailable' };
	}

	/** Asks for a code. The answer is the same whoever the name is, so it always lands on the log. */
	async function ask(): Promise<boolean> {
		busy = true;
		problem = null;
		try {
			await library.requestReset(name);
		} catch (error) {
			busy = false;
			problem = failed(error);
			if (problem.kind === 'throttled') {
				holdOff(problem.retryAfterSeconds);
			}
			return false;
		}
		busy = false;
		askedAt = Date.now();
		now = askedAt;
		return true;
	}

	async function submitName(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		if (busy || waiting > 0 || name.trim() === '') {
			return;
		}
		if (await ask()) {
			step = 'sent';
		}
	}

	async function askAgain(): Promise<void> {
		if (busy || waiting > 0) {
			return;
		}
		if (await ask()) {
			code = '';
			await tick();
			codeField?.focus();
		}
	}

	async function haveCode(): Promise<void> {
		step = 'code';
		problem = null;
		await tick();
		codeField?.focus();
	}

	async function differentName(): Promise<void> {
		step = 'name';
		problem = null;
		await tick();
		nameField?.focus();
		nameField?.select();
	}

	/**
	 * A wrong or stale code keeps the passwords, since nothing is wrong with them, and the code takes
	 * the focus, selected. A password problem clears both and the first takes it; the code stays.
	 */
	async function refused(next: ResetProblem): Promise<void> {
		busy = false;
		problem = next;
		if (next.kind === 'throttled') {
			holdOff(next.retryAfterSeconds);
		}
		await tick();
		if (resetFieldOf(next) === 'code') {
			codeField?.focus();
			codeField?.select();
			return;
		}
		if (resetFieldOf(next) === 'password') {
			for (const field of [passwordField, againField]) {
				if (field !== undefined) {
					field.value = '';
				}
			}
			passwordField?.focus();
		}
	}

	async function submitCode(
		event: SubmitEvent & { currentTarget: HTMLFormElement }
	): Promise<void> {
		event.preventDefault();
		if (busy || waiting > 0) {
			return;
		}
		if (expired) {
			await askAgain();
			return;
		}
		const fields = new FormData(event.currentTarget);
		const password = String(fields.get('password'));
		const own = checkPasswords(password, String(fields.get('again')));
		if (own !== null) {
			await refused(own);
			return;
		}
		busy = true;
		problem = null;
		try {
			await library.confirmReset(name, code, password);
		} catch (error) {
			await refused(resetRefusal(failed(error), askedAt, Date.now()));
			return;
		}
		step = 'done';
		await ondone();
	}

	/** Keeps the code in the log's groups as it's typed or pasted, whatever its case or dashes. */
	function typeCode(event: Event & { currentTarget: HTMLInputElement }): void {
		const shown = formatCode(event.currentTarget.value);
		if (shown !== event.currentTarget.value) {
			event.currentTarget.value = shown;
		}
		code = shown;
		if (problem?.kind === 'code' || problem?.kind === 'expired') {
			problem = null;
		}
	}

	async function copyCommand(): Promise<void> {
		try {
			await navigator.clipboard.writeText(findCodeCommand.replaceAll('\\\n  ', ''));
		} catch {
			return; // No clipboard here: the command is on the screen to copy by hand.
		}
		copied = true;
		setTimeout(() => (copied = false), 2000);
	}

	onMount(() => {
		nameField?.focus();
		nameField?.select();
		clock = setInterval(() => (now = Date.now()), 5000);
	});

	onDestroy(() => {
		clearInterval(countdown);
		clearInterval(clock);
	});
</script>

{#key step}
	<div class="card" in:fly={{ y: 16, duration: still ? 0 : 280 }}>
		{#if step === 'name'}
			<form onsubmit={submitName}>
				<p class="mono">RESET YOUR PASSWORD · 1 OF 2</p>
				<h1>What's your name?</h1>
				<p class="lede">
					Kitsune sends no email. Instead it puts a reset code in the server's log, where whoever
					runs Kitsune can read it.
				</p>
				<label>
					<span>Name</span>
					<input
						name="name"
						autocomplete="username"
						autocapitalize="none"
						autocorrect="off"
						spellcheck="false"
						required
						bind:value={name}
						bind:this={nameField}
					/>
				</label>
				<button type="submit" class="solid" disabled={busy || waiting > 0} aria-busy={busy}>
					{busy ? 'Asking…' : 'Get a reset code'}
				</button>
				<p class="problem" role="alert">{message}</p>
				<button type="button" class="quiet" onclick={() => onback(name)}>Back to sign in</button>
			</form>
		{:else if step === 'sent'}
			<div class="form">
				<p class="mono">RESET YOUR PASSWORD · 1 OF 2</p>
				<h1>Check the log<span class="period">.</span></h1>
				<p class="lede">
					If that account exists, a reset code is now in the server's log. Whoever runs Kitsune can
					find it there.
				</p>
				<details open>
					<summary>How to find it<span class="caret" aria-hidden="true">▾</span></summary>
					<div class="how">
						<p>On the machine that runs Kitsune:</p>
						<pre><code>{findCodeCommand}</code></pre>
						<button type="button" class="chip" onclick={() => void copyCommand()}>
							{copied ? 'Copied' : 'Copy the command'}
						</button>
						<p>
							The newest line with your name holds the code, in four groups like
							<code class="sample">k7m2-q9xd-4tbn-r8ve</code>. It lasts <strong>15 minutes</strong>.
							Asking again makes a new one, and the old one stops working.
						</p>
					</div>
				</details>
				<button type="button" class="solid" onclick={() => void haveCode()}>I have the code</button>
				<button type="button" class="quiet" onclick={() => void differentName()}
					>Use a different name</button
				>
			</div>
		{:else if step === 'code'}
			<form onsubmit={submitCode}>
				<p class="mono">RESET YOUR PASSWORD · 2 OF 2</p>
				<h1>A new password for {name}</h1>
				<label>
					<span class="label-row">
						<span>Reset code</span>
						<span class="mono left" aria-live="polite">{timeLeftLine(askedAt, now)}</span>
					</span>
					<input
						class="code"
						name="code"
						value={code}
						placeholder="xxxx-xxxx-xxxx-xxxx"
						autocomplete="one-time-code"
						autocapitalize="none"
						autocorrect="off"
						spellcheck="false"
						required
						aria-invalid={wrong === 'code' || expired}
						aria-describedby="code-hint"
						oninput={typeCode}
						bind:this={codeField}
					/>
				</label>
				{#if expired}
					<p class="problem" role="alert">{resetProblemLine({ kind: 'expired' }, 0)}</p>
					<button type="submit" class="ghost" disabled={busy || waiting > 0} aria-busy={busy}>
						{busy ? 'Asking…' : 'Get a new code'}
					</button>
					<p class="hint">The passwords you typed stay, ready for the new code.</p>
				{:else}
					<p class="hint" id="code-hint">From the log. Dashes and capitals don't matter.</p>
				{/if}
				<hr />
				<label>
					<span>New password</span>
					<input
						name="password"
						type="password"
						autocomplete="new-password"
						required
						aria-invalid={wrong === 'password'}
						bind:this={passwordField}
					/>
				</label>
				<label>
					<span>New password again</span>
					<input
						name="again"
						type="password"
						autocomplete="new-password"
						required
						bind:this={againField}
					/>
				</label>
				{#if !expired}
					<button type="submit" class="solid" disabled={busy || waiting > 0} aria-busy={busy}>
						{busy ? 'Saving…' : 'Save and sign in'}
					</button>
					<p class="problem" role="alert">{message}</p>
					<p class="hint" hidden={message !== ''}>
						At least 8 characters. Anywhere else you're signed in, you'll be signed out.
					</p>
					{#if problem?.kind === 'throttled'}
						<p class="hint">Your code keeps working until its 15 minutes are up.</p>
					{/if}
					<button
						type="button"
						class="quiet"
						disabled={busy || waiting > 0}
						onclick={() => void askAgain()}>Ask for a new code</button
					>
				{/if}
			</form>
		{:else}
			<div class="form">
				<p class="mono">RESET YOUR PASSWORD · DONE</p>
				<h1>Password changed<span class="period">.</span></h1>
				<p class="lede">Anywhere else you were signed in now needs the new one.</p>
				<button type="button" class="solid" disabled aria-busy="true">Signing you in…</button>
			</div>
		{/if}
	</div>
{/key}

<style>
	.card {
		width: 100%;
		padding: var(--reset-pad, 24px);
		border-radius: var(--radius-28);
		background: var(--card);
		border: 1px solid var(--edge-card);
	}

	form,
	.form {
		display: flex;
		flex-direction: column;
		gap: 16px;
	}

	.mono {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	h1 {
		font-size: var(--reset-title, 28px);
		font-weight: 800;
		letter-spacing: -0.03em;
		line-height: 1.1;
		overflow-wrap: anywhere;
	}

	.period {
		color: var(--fox-text);
	}

	.lede {
		font-size: var(--type-body);
		line-height: 1.5;
		color: var(--text-secondary);
	}

	label {
		display: flex;
		flex-direction: column;
		gap: 8px;
		font-size: 14px;
		font-weight: 600;
		color: var(--text-secondary);
	}

	.label-row {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 12px;
	}

	.left {
		font-weight: 400;
		font-size: 11px;
	}

	input {
		height: 52px;
		padding: 0 20px;
		border: 1px solid var(--edge-strong);
		border-radius: var(--radius-pill);
		background: var(--input);
		color: var(--text);
		caret-color: var(--fox-line);
		/* Under 16 px, iOS zooms the page on focus. */
		font: 400 16px var(--font-sans);
		transition: border-color var(--morph-fast) var(--curve-morph);
	}

	input:hover {
		border-color: var(--edge-hover);
	}

	/* A code reads best a character at a time. */
	input.code {
		font-family: var(--font-mono);
		font-size: 17px;
		letter-spacing: 0.04em;
	}

	input[aria-invalid='true'] {
		border-color: var(--fox-line);
	}

	input:-webkit-autofill {
		-webkit-text-fill-color: var(--text);
		box-shadow: inset 0 0 0 52px var(--input-solid);
		caret-color: var(--fox-line);
	}

	hr {
		border: 0;
		border-top: 1px solid var(--edge-card);
		margin: 0;
	}

	button {
		height: 52px;
		border-radius: var(--radius-pill);
		font: 700 16px var(--font-sans);
		transition:
			background var(--morph-fast) var(--curve-morph),
			border-color var(--morph-fast) var(--curve-morph);
	}

	.solid {
		margin-top: 8px;
		border: 0;
		background: var(--fox);
		color: var(--fox-ink);
	}

	.solid:disabled {
		background: color-mix(in srgb, var(--fox) 45%, var(--ground));
		cursor: default;
	}

	.ghost {
		border: 1px solid var(--edge-strong);
		background: transparent;
		color: var(--text);
		font-size: 14px;
		font-weight: 600;
	}

	.quiet {
		height: var(--target);
		border: 0;
		background: transparent;
		color: var(--text-secondary);
		font-size: 14px;
		font-weight: 600;
	}

	.quiet:hover:not(:disabled) {
		color: var(--fox-text);
	}

	.quiet:disabled {
		opacity: 0.6;
		cursor: default;
	}

	details {
		border-radius: 20px;
		background: var(--input);
		border: 1px solid var(--edge-card);
	}

	summary {
		height: var(--target);
		padding: 0 16px;
		display: flex;
		align-items: center;
		justify-content: space-between;
		list-style: none;
		cursor: pointer;
		font-size: 14px;
		font-weight: 600;
	}

	summary::-webkit-details-marker {
		display: none;
	}

	.caret {
		color: var(--text-meta);
		font-size: 12px;
		transition: rotate var(--morph-fast) var(--curve-morph);
	}

	details[open] .caret {
		rotate: 180deg;
	}

	.how {
		padding: 0 16px 16px;
		display: flex;
		flex-direction: column;
		gap: 10px;
		font-size: 13px;
		line-height: 1.5;
		color: var(--text-secondary);
	}

	pre {
		margin: 0;
		padding: 10px 12px;
		border-radius: 14px;
		background: var(--ground);
		border: 1px solid var(--edge-card);
		overflow-x: auto;
	}

	code {
		font-family: var(--font-mono);
		font-size: 12px;
		line-height: 1.6;
		color: var(--text);
	}

	.sample {
		white-space: nowrap;
	}

	strong {
		color: var(--text);
	}

	.chip {
		align-self: flex-start;
		height: 36px;
		padding: 0 14px;
		border: 0;
		background: var(--input);
		color: var(--text);
		font-size: 13px;
		font-weight: 600;
	}

	.hint {
		font-size: 13px;
		line-height: 1.5;
		color: var(--text-label);
	}

	.problem {
		font-size: 14px;
		line-height: 1.45;
		color: var(--fox-text);
	}

	.problem:empty {
		display: none;
	}
</style>
