<script lang="ts">
	import { onDestroy, tick } from 'svelte';
	import { library } from '#lib/library/library.svelte.ts';
	import { PasswordError, UnauthorizedError } from '#lib/library/source.ts';
	import { passwordProblemLine, type PasswordProblem } from './passwords.ts';
	import { checkPasswords } from './setup.ts';

	/** The You page's HOW YOU SIGN IN: change your password, staying signed in here. */

	let busy = $state(false);
	let problem = $state.raw<PasswordProblem | null>(null);
	let done = $state(0);
	/** Seconds until the server will listen again, while it's refusing. */
	let waiting = $state(0);
	let countdown: ReturnType<typeof setInterval> | undefined;

	let currentField: HTMLInputElement | undefined = $state();
	let passwordField: HTMLInputElement | undefined = $state();
	let againField: HTMLInputElement | undefined = $state();

	const message = $derived(passwordProblemLine(problem, waiting));
	const wrongCurrent = $derived(problem?.kind === 'current');
	const wrongNew = $derived(
		problem?.kind === 'short' || problem?.kind === 'mismatch' || problem?.kind === 'password'
	);

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

	function clear(...fields: (HTMLInputElement | undefined)[]): void {
		for (const field of fields) {
			if (field !== undefined) {
				field.value = '';
			}
		}
	}

	/**
	 * A wrong current password clears only that field; a problem with the new one clears both new
	 * fields, and the first takes the focus.
	 */
	async function refused(next: PasswordProblem): Promise<void> {
		busy = false;
		problem = next;
		if (next.kind === 'throttled') {
			holdOff(next.retryAfterSeconds);
		}
		await tick();
		if (next.kind === 'current') {
			clear(currentField);
			currentField?.focus();
		} else if (next.kind !== 'throttled' && next.kind !== 'unavailable') {
			clear(passwordField, againField);
			passwordField?.focus();
		}
	}

	async function submit(event: SubmitEvent & { currentTarget: HTMLFormElement }): Promise<void> {
		event.preventDefault();
		if (busy || waiting > 0) {
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
			await library.changePassword(String(fields.get('current')), password);
		} catch (error) {
			if (error instanceof UnauthorizedError) {
				return; // The door has it from here.
			}
			await refused(error instanceof PasswordError ? error.failure : { kind: 'unavailable' });
			return;
		}
		busy = false;
		clear(currentField, passwordField, againField);
		done += 1;
	}

	onDestroy(() => {
		clearInterval(countdown);
	});
</script>

<section aria-labelledby="signing-in" class="signing" data-morph-part="panel">
	<div class="head">
		<h2 id="signing-in" class="mono">HOW YOU SIGN IN</h2>
		{#if done > 0}
			{#key done}
				<p class="said" role="status">Password changed · other devices signed out</p>
			{/key}
		{/if}
	</div>
	<form class="card" onsubmit={submit}>
		<h3>Password</h3>
		<!-- The name is there for password managers, which file the new password under it. -->
		<input
			class="hidden"
			name="username"
			autocomplete="username"
			value={library.user?.name ?? ''}
			readonly
			tabindex="-1"
			aria-hidden="true"
		/>
		<label>
			<span>Current password</span>
			<input
				name="current"
				type="password"
				autocomplete="current-password"
				required
				aria-invalid={wrongCurrent}
				bind:this={currentField}
			/>
		</label>
		<div class="pair">
			<label>
				<span>New password</span>
				<input
					name="password"
					type="password"
					autocomplete="new-password"
					required
					aria-invalid={wrongNew}
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
		</div>
		<p class="problem" role="alert">{message}</p>
		<div class="foot">
			<p class="hint">
				At least 8 characters. Your other devices get signed out; this one stays signed in.
			</p>
			<button type="submit" disabled={busy || waiting > 0} aria-busy={busy}>
				{busy ? 'Changing…' : 'Change password'}
			</button>
		</div>
	</form>
</section>

<style>
	.signing {
		margin-top: 112px;
	}

	.head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
		/* The line comes and goes without moving the card. */
		height: 16px;
	}

	.mono {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		font-weight: 400;
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	/* Comes in, stays, and goes on its own, like Last.fm's line. */
	.said {
		font-size: 13px;
		color: var(--fox-text);
		animation: said 4200ms var(--curve-morph) forwards;
	}

	@keyframes said {
		0% {
			opacity: 0;
			translate: 0 4px;
		}

		8%,
		82% {
			opacity: 1;
			translate: 0 0;
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
		display: flex;
		flex-direction: column;
		gap: 16px;
	}

	h3 {
		margin: 0 0 4px;
		font-size: var(--type-section);
		font-weight: 800;
		letter-spacing: var(--tracking-section);
	}

	.hidden {
		display: none;
	}

	label {
		display: flex;
		flex-direction: column;
		gap: 8px;
		min-width: 0;
		font-size: 14px;
		font-weight: 600;
		color: var(--text-secondary);
	}

	.pair {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 16px;
	}

	input {
		height: 52px;
		padding: 0 20px;
		border: 1px solid var(--edge-strong);
		border-radius: var(--radius-pill);
		background: var(--input);
		color: var(--text);
		caret-color: var(--fox-line);
		font: 400 16px var(--font-sans);
		transition: border-color var(--morph-fast) var(--curve-morph);
	}

	input:hover {
		border-color: var(--edge-hover);
	}

	input[aria-invalid='true'] {
		border-color: var(--fox-line);
	}

	input:-webkit-autofill {
		-webkit-text-fill-color: var(--text);
		box-shadow: inset 0 0 0 52px var(--input-solid);
		caret-color: var(--fox-line);
	}

	.foot {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 24px;
	}

	.hint {
		font-size: 13px;
		line-height: 1.5;
		color: var(--text-label);
	}

	button {
		flex: none;
		height: 48px;
		padding: 0 22px;
		border: 0;
		border-radius: var(--radius-pill);
		background: var(--fox);
		color: var(--fox-ink);
		font-size: 14px;
		font-weight: 700;
		transition: background var(--morph-fast) var(--curve-morph);
	}

	button:disabled {
		background: color-mix(in srgb, var(--fox) 45%, var(--ground));
		cursor: default;
	}

	.problem {
		font-size: 14px;
		line-height: 1.45;
		color: var(--fox-text);
	}

	.problem:empty {
		display: none;
	}

	@media (prefers-reduced-motion: reduce) {
		.said {
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
	}

	@media (max-width: 1100px) {
		.signing {
			margin-top: 56px;
		}
	}

	@media (max-width: 720px) {
		.card {
			padding: 20px;
		}

		.pair {
			grid-template-columns: minmax(0, 1fr);
		}

		.foot {
			flex-direction: column;
			align-items: stretch;
			gap: 16px;
		}

		button {
			height: 52px;
		}
	}
</style>
