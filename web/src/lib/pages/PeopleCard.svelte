<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { resolve } from '$app/paths';
	import { library } from '#lib/library/library.svelte.ts';
	import { PasswordError, type Account } from '#lib/library/source.ts';
	import {
		lastSeenLine,
		passwordProblemLine,
		roleWords,
		signsOutLine,
		type PasswordProblem
	} from './passwords.ts';
	import { checkPasswords } from './setup.ts';

	/** The Server page's People: every account, and a new password for anyone but you. */

	let accounts = $state.raw<Account[] | null>(null);
	let unreadable = $state(false);
	let now = $state(Date.now());
	/** The account whose popover is open. */
	let open = $state<number | null>(null);
	let busy = $state(false);
	let problem = $state.raw<PasswordProblem | null>(null);
	/** Whose password was just set, said for a moment. */
	let said = $state<{ name: string; n: number } | null>(null);
	let passwordField: HTMLInputElement | undefined = $state();
	let againField: HTMLInputElement | undefined = $state();

	const me = $derived(library.user?.id);
	const message = $derived(passwordProblemLine(problem, 0));

	async function read(): Promise<void> {
		try {
			// You come first, as the board has it: the one row that sends you elsewhere.
			const all = await library.accounts();
			accounts = all.toSorted((a, b) => Number(b.user.id === me) - Number(a.user.id === me));
			unreadable = false;
		} catch {
			unreadable = true;
		}
		now = Date.now();
	}

	async function toggle(id: number): Promise<void> {
		problem = null;
		open = open === id ? null : id;
		if (open !== null) {
			await tick();
			passwordField?.focus();
		}
	}

	function close(): void {
		const was = open;
		open = null;
		problem = null;
		document.getElementById(`set-${String(was)}`)?.focus();
	}

	async function submit(
		account: Account,
		event: SubmitEvent & { currentTarget: HTMLFormElement }
	): Promise<void> {
		event.preventDefault();
		if (busy) {
			return;
		}
		const fields = new FormData(event.currentTarget);
		const password = String(fields.get('password'));
		const own = checkPasswords(password, String(fields.get('again')));
		if (own === null) {
			busy = true;
			problem = null;
			try {
				await library.setPassword(account.user.id, password);
			} catch (error) {
				problem = error instanceof PasswordError ? error.failure : { kind: 'unavailable' };
			}
			busy = false;
		} else {
			problem = own;
		}
		if (problem !== null) {
			for (const field of [passwordField, againField]) {
				if (field !== undefined) {
					field.value = '';
				}
			}
			passwordField?.focus();
			return;
		}
		said = { name: account.user.name, n: (said?.n ?? 0) + 1 };
		close();
		await read();
	}

	function onkeydown(event: KeyboardEvent): void {
		if (event.key === 'Escape' && open !== null) {
			event.preventDefault();
			close();
		}
	}

	onMount(() => {
		void read();
		const clock = setInterval(() => (now = Date.now()), 60_000);
		return () => {
			clearInterval(clock);
		};
	});
</script>

<svelte:window {onkeydown} />

<section
	aria-labelledby="people-title"
	class="people"
	data-morph-part="panel"
	data-morph-key="people"
>
	<div class="head">
		<h2 id="people-title">People</h2>
		{#if said !== null}
			{#key said.n}
				<p class="said" role="status">{said.name} has a new password · signed out everywhere</p>
			{/key}
		{:else if accounts !== null}
			<span class="mono count"
				>{accounts.length} {accounts.length === 1 ? 'ACCOUNT' : 'ACCOUNTS'}</span
			>
		{/if}
	</div>

	{#if unreadable}
		<p class="note" role="status">
			The accounts couldn’t be read.
			<button type="button" class="link" onclick={() => void read()}>Try again</button>
		</p>
	{:else if accounts !== null}
		<div class="table" role="table" aria-label="Accounts">
			<div class="row header" role="row">
				<span role="columnheader" class="mono name-head">NAME</span>
				<span role="columnheader" class="mono">ROLE</span>
				<span role="columnheader" class="mono">LAST SEEN</span>
				<span role="columnheader"><span class="sr">Password</span></span>
			</div>
			{#each accounts as account (account.user.id)}
				{@const yours = account.user.id === me}
				<div class="row" role="row">
					<span role="cell" class="who">
						<span class="initial" aria-hidden="true"
							>{account.user.name.slice(0, 1).toUpperCase()}</span
						>
						<span class="name">{account.user.name}</span>
						{#if yours}<span class="mono you">YOU</span>{/if}
					</span>
					<span role="cell" class="role">{roleWords(account.user.role)}</span>
					<span role="cell" class="seen">{lastSeenLine(account, now)}</span>
					<span role="cell" class="act">
						{#if yours}
							<a class="yours" href={resolve('/you')}
								>Change yours on You <span aria-hidden="true">›</span></a
							>
						{:else}
							<button
								type="button"
								class="set"
								id={`set-${String(account.user.id)}`}
								aria-expanded={open === account.user.id}
								aria-controls={`pop-${String(account.user.id)}`}
								onclick={() => void toggle(account.user.id)}>Set a new password…</button
							>
							{#if open === account.user.id}
								<div
									class="pop"
									id={`pop-${String(account.user.id)}`}
									role="dialog"
									aria-label={`A new password for ${account.user.name}`}
								>
									<form onsubmit={(event) => void submit(account, event)}>
										<div class="pop-title">A new password for {account.user.name}</div>
										<p class="pop-body">{signsOutLine(account)}</p>
										<label>
											<span>New password</span>
											<input
												name="password"
												type="password"
												autocomplete="new-password"
												required
												aria-invalid={problem !== null}
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
										{#if message === ''}
											<p class="hint">At least 8 characters.</p>
										{:else}
											<p class="problem" role="alert">{message}</p>
										{/if}
										<div class="buttons">
											<button type="button" class="cancel" onclick={close}>Cancel</button>
											<button type="submit" class="solid" disabled={busy} aria-busy={busy}>
												{busy ? 'Setting…' : 'Set password'}
											</button>
										</div>
									</form>
								</div>
							{/if}
						{/if}
					</span>
				</div>
			{/each}
		</div>
		<p class="foot">
			Setting someone’s password signs them out everywhere. Kitsune doesn’t send it to them: tell
			them yourself. Anyone who forgot theirs can also reset it from the sign-in page, with a code
			from the server’s log.
		</p>
	{/if}
</section>

<style>
	.people {
		box-sizing: border-box;
		min-width: 0;
		padding: var(--section-pad);
		border-radius: var(--section-radius);
		background: var(--card);
		border: 1px solid var(--edge-card);
		display: flex;
		flex-direction: column;
		gap: var(--section-gap);
	}

	.head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
	}

	h2 {
		font-size: var(--section-title);
		font-weight: 800;
		letter-spacing: var(--tracking-section);
	}

	.mono {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		font-weight: 400;
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.said {
		font-size: 13px;
		color: var(--fox-text);
		animation: said 4200ms var(--curve-morph) forwards;
	}

	@keyframes said {
		0% {
			opacity: 0;
		}

		8%,
		82% {
			opacity: 1;
		}

		100% {
			opacity: 0;
		}
	}

	/* One grid for every row, so the columns line up whatever each row holds. */
	.table {
		display: grid;
		grid-template-columns: minmax(0, 3fr) minmax(0, 2fr) minmax(0, 3fr) auto;
		row-gap: 8px;
	}

	/* The columns stretch with the card: nothing here is a fixed column centred in the page. */
	.row {
		position: relative;
		min-height: 64px;
		padding: 0 12px;
		border-radius: 16px;
		background: var(--card-raised);
		grid-column: 1 / -1;
		display: grid;
		grid-template-columns: subgrid;
		align-items: center;
		column-gap: 16px;
	}

	.row.header {
		min-height: 0;
		background: transparent;
	}

	.name-head {
		padding-left: 48px;
	}

	.sr {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip-path: inset(50%);
	}

	.who {
		min-width: 0;
		display: flex;
		align-items: center;
		gap: 12px;
	}

	.initial {
		flex: none;
		width: 36px;
		height: 36px;
		border-radius: 50%;
		background: var(--input);
		color: var(--text-secondary);
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 14px;
		font-weight: 700;
	}

	.name {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: 17px;
		font-weight: 700;
	}

	.you {
		color: var(--fox-text);
		font-size: 11px;
	}

	.role {
		font-size: 14px;
		color: var(--text-secondary);
	}

	.seen {
		font-family: var(--font-mono);
		font-size: 12px;
		color: var(--text-meta);
	}

	.act {
		justify-self: end;
	}

	.yours {
		height: var(--target);
		padding: 0 4px;
		display: inline-flex;
		align-items: center;
		gap: 6px;
		color: var(--text);
		font-size: 13px;
		font-weight: 600;
		text-decoration: none;
	}

	.yours:hover {
		color: var(--fox-text);
	}

	button {
		border-radius: var(--radius-pill);
		font-weight: 600;
		transition:
			background var(--morph-fast) var(--curve-morph),
			border-color var(--morph-fast) var(--curve-morph);
	}

	.set {
		height: var(--target);
		padding: 0 16px;
		border: 1px solid var(--edge-strong);
		background: transparent;
		color: var(--text);
		font-size: 13px;
	}

	.set:hover,
	.set[aria-expanded='true'] {
		border-color: var(--fox-line);
	}

	/* Anchored to the row's button, under it and to its right edge. */
	.pop {
		position: absolute;
		z-index: 5;
		top: calc(100% - 4px);
		right: -28px;
		width: min(360px, calc(100vw - 32px));
		padding: 20px;
		border-radius: 24px;
		background: var(--card-raised);
		border: 1px solid var(--edge-strong);
		box-shadow: var(--glass-shadow);
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	.pop form {
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	.pop-title {
		font-size: 17px;
		font-weight: 800;
		letter-spacing: -0.02em;
	}

	.pop-body,
	.hint {
		font-size: 13px;
		line-height: 1.5;
		color: var(--text-label);
	}

	.pop-body {
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

	input {
		height: 48px;
		padding: 0 20px;
		border: 1px solid var(--edge-strong);
		border-radius: var(--radius-pill);
		background: var(--input);
		color: var(--text);
		caret-color: var(--fox-line);
		font: 400 16px var(--font-sans);
	}

	input[aria-invalid='true'] {
		border-color: var(--fox-line);
	}

	.buttons {
		display: flex;
		justify-content: flex-end;
		gap: 8px;
	}

	.cancel {
		height: var(--target);
		padding: 0 16px;
		border: 0;
		background: transparent;
		color: var(--text);
		font-size: 14px;
	}

	.solid {
		height: var(--target);
		padding: 0 20px;
		border: 0;
		background: var(--fox);
		color: var(--fox-ink);
		font-size: 14px;
		font-weight: 700;
	}

	.solid:disabled {
		background: color-mix(in srgb, var(--fox) 45%, var(--ground));
		cursor: default;
	}

	.problem {
		font-size: 13px;
		line-height: 1.45;
		color: var(--fox-text);
	}

	.foot,
	.note {
		max-width: 720px;
		font-size: 13px;
		line-height: 1.55;
		color: var(--text-meta);
	}

	.link {
		border: 0;
		background: transparent;
		color: var(--fox-text);
		text-decoration: underline;
	}

	@media (min-width: 3000px) {
		.row {
			min-height: 80px;
			padding: 0 16px;
		}

		.name {
			font-size: 21px;
		}

		.role,
		.set,
		.yours,
		.foot {
			font-size: 17px;
		}

		.seen {
			font-size: 15px;
		}

		.initial {
			width: 44px;
			height: 44px;
			font-size: 17px;
		}

		.name-head {
			padding-left: 60px;
		}
	}

	/* On a phone each person is a small card: name and role over when, the button under. */
	@media (max-width: 720px) {
		.table {
			grid-template-columns: minmax(0, 1fr);
		}

		.row {
			padding: 14px;
			grid-template-columns: minmax(0, 1fr);
			row-gap: 6px;
		}

		.row.header {
			display: none;
		}

		.who {
			grid-column: 1 / -1;
		}

		.role,
		.seen {
			padding-left: 48px;
			grid-column: 1 / -1;
		}

		.act {
			grid-column: 1 / -1;
			justify-self: stretch;
			padding-left: 48px;
		}

		.set {
			width: 100%;
		}

		.pop {
			left: 0;
			right: 0;
			width: auto;
		}
	}
</style>
