<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { library } from '#lib/library/library.svelte.ts';
	import { safeNext } from '#lib/library/session.ts';
	import { LoginError } from '#lib/library/source.ts';

	let message = $state('');

	async function submit(event: SubmitEvent & { currentTarget: HTMLFormElement }): Promise<void> {
		event.preventDefault();
		const form = event.currentTarget;
		const data = new FormData(form);
		try {
			await library.login(String(data.get('name')), String(data.get('password')));
		} catch (error) {
			message = error instanceof LoginError ? error.failure.kind : 'unavailable';
			return;
		}
		await library.loadShell();
		const next = safeNext(page.url.searchParams.get('next'), location.origin);
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- `safeNext` returns a path on this origin.
		await goto(next, { replaceState: true });
	}
</script>

<svelte:head>
	<title>Sign in · Kitsune</title>
</svelte:head>

<form onsubmit={submit}>
	<label>Name <input name="name" autocomplete="username" autocapitalize="none" required /></label>
	<label
		>Password <input
			name="password"
			type="password"
			autocomplete="current-password"
			required
		/></label
	>
	<button type="submit">Sign in</button>
	<p>{message}</p>
</form>
