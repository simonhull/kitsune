<script lang="ts">
	import { onDestroy } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { library } from '#lib/library/library.svelte.ts';
	import { safeNext } from '#lib/library/session.ts';
	import { LoginError, type LoginFailure } from '#lib/library/source.ts';
	import { morph } from '#lib/morph/registry.ts';
	import Icon from '#lib/shell/Icon.svelte';

	let busy = $state(false);
	let failure = $state.raw<LoginFailure | null>(null);
	/** Seconds until the server will listen again, while it's refusing. */
	let waiting = $state(0);
	let password: HTMLInputElement | undefined = $state();
	let countdown: ReturnType<typeof setInterval> | undefined;

	const message = $derived.by(() => {
		switch (failure?.kind) {
			case 'credentials':
				return "That name and password don't match.";
			case 'throttled': {
				const minutes = Math.max(1, Math.ceil(waiting / 60));
				return `Too many tries. Try again in ${String(minutes)} ${minutes === 1 ? 'minute' : 'minutes'}.`;
			}
			case 'unavailable':
				return "Couldn't reach Kitsune. Try again in a moment.";
			default:
				return '';
		}
	});

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
				failure = null;
			}
		}, 1000);
	}

	/** The name stays, because it's usually right; the password goes. */
	function refused(error: unknown): void {
		failure = error instanceof LoginError ? error.failure : { kind: 'unavailable' };
		if (failure.kind === 'throttled') {
			holdOff(failure.retryAfterSeconds);
		}
		busy = false;
		if (password !== undefined) {
			password.value = '';
			password.focus();
		}
	}

	async function submit(event: SubmitEvent & { currentTarget: HTMLFormElement }): Promise<void> {
		event.preventDefault();
		if (busy || waiting > 0) {
			return;
		}
		const fields = new FormData(event.currentTarget);
		busy = true;
		failure = null;
		try {
			await library.login(String(fields.get('name')), String(fields.get('password')));
		} catch (error) {
			refused(error);
			return;
		}
		// Home needs its units on the page before the fox leaves, or it has nothing to arrive.
		await library.loadShell();
		const next = safeNext(page.url.searchParams.get('next'), location.origin);
		const [, albumId] = /^\/album\/([^/?]+)/.exec(next) ?? [];
		if (albumId !== undefined) {
			const id = decodeURIComponent(albumId);
			await library.loadAlbum(id);
			// The island forms on this journey, so it forms already holding the album it's going to.
			const album = library.album(id);
			if (album !== undefined) {
				library.cueAlbum(album);
			}
		}
		// Replacing, so Back never returns to the door.
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- `safeNext` returns a path on this origin.
		await goto(next, { replaceState: true });
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
	<title>Sign in · Kitsune</title>
</svelte:head>

<div class="door">
	<div class="column">
		<h1>Sign in to Kitsune</h1>
		<span class="fox" use:morph={{ id: 'fox', slot: 'door', kind: 'control' }}
			><Icon name="fox" size={44} /></span
		>
		<form data-morph-part="words" onsubmit={submit}>
			<label>
				<span>Name</span>
				<input
					name="name"
					autocomplete="username"
					autocapitalize="none"
					autocorrect="off"
					spellcheck="false"
					required
					use:focusOnDesktop
				/>
			</label>
			<label>
				<span>Password</span>
				<input
					name="password"
					type="password"
					autocomplete="current-password"
					required
					bind:this={password}
				/>
			</label>
			<button type="submit" disabled={busy || waiting > 0} aria-busy={busy}>
				{busy ? 'Signing in…' : 'Sign in'}
			</button>
			<p class="error" role="alert">{message}</p>
		</form>
	</div>
</div>

<style>
	.door {
		min-height: 100dvh;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 24px 16px;
	}

	.column {
		width: min(340px, 100%);
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 32px;
	}

	h1 {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip-path: inset(50%);
		white-space: nowrap;
	}

	/* The nav's mark at twice the size, so the flight home is one shape shrinking. */
	.fox {
		flex: none;
		width: 88px;
		height: 88px;
		border-radius: 50%;
		background: var(--fox);
		color: var(--fox-ink);
		display: flex;
		align-items: center;
		justify-content: center;
	}

	form {
		width: 100%;
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
		caret-color: var(--fox-line);
		/* Under 16 px, iOS zooms the page on focus. */
		font: 400 16px var(--font-sans);
		transition: border-color var(--morph-fast) var(--curve-morph);
	}

	input:hover {
		border-color: var(--edge-hover);
	}

	/* The browser's yellow has no place here. */
	input:-webkit-autofill {
		-webkit-text-fill-color: var(--text);
		box-shadow: inset 0 0 0 52px var(--input-solid);
		caret-color: var(--fox-line);
	}

	button {
		height: 52px;
		margin-top: 8px;
		border: 0;
		border-radius: var(--radius-pill);
		background: var(--fox);
		color: var(--fox-ink);
		font-size: 16px;
		font-weight: 700;
		transition: background var(--morph-fast) var(--curve-morph);
	}

	button:disabled {
		background: color-mix(in srgb, var(--fox) 45%, var(--ground));
		cursor: default;
	}

	/* Its line is always there, so a refusal doesn't push the form around. */
	.error {
		min-height: 40px;
		font-size: 14px;
		line-height: 20px;
		text-align: center;
		color: var(--fox-text);
	}
</style>
