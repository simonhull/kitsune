<!--
	Export .m3u, in place of a playlist's buttons, as Delete's confirm is: a head that says it's a
	snapshot, with Leave it, then two cards. Download is a plain link the browser saves. Save in the
	library, for an administrator with tag writes on and never on a phone, writes the file into a
	library folder: when one is there already the card turns fox-bordered and asks (Replace it, Save
	as the next free name, Keep it), and nothing is overwritten without that; once written it turns
	fox-washed and says so.
-->
<script lang="ts">
	import { reshapeOn } from '#lib/morph/reshapeOn.svelte.ts';
	import { SurfaceSlot } from '#lib/morph/surface.ts';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { library } from '#lib/library/library.svelte.ts';
	import {
		PlaylistExistsError,
		type ExistingFile,
		type PlaylistSaved,
		type PlaylistSummary
	} from '#lib/library/source.ts';
	import FolderPicker from './FolderPicker.svelte';
	import {
		downloadLine,
		existsWords,
		exportFileName,
		exportHead,
		folderLabel,
		saveAsWords,
		savedWords,
		underRoot
	} from './fileWords.ts';

	interface Props {
		playlist: PlaylistSummary;
		phone: boolean;
		onleave: () => void;
	}

	let { playlist, phone, onleave }: Props = $props();

	/** Saving is an administrator's, with tag writes on, and waits for a desktop. */
	const canSave = $derived(
		!phone && library.user?.role === 'admin' && library.user.canTrash === true
	);

	let roots = $state.raw<string[]>([]);
	let root = $state('');
	let folder = $state('Playlists');
	/** The file's name: the playlist's, sanitized, until it's typed over. */
	let name = $derived(exportFileName(playlist.name));
	let picking = $state(false);
	let step = $state<'choose' | 'confirm' | 'saved'>('choose');
	let exists = $state.raw<ExistingFile | null>(null);
	let saved = $state.raw<PlaylistSaved | null>(null);
	let busy = $state(false);
	let trouble = $state('');
	const now = Date.now();

	$effect(() => {
		if (!canSave) {
			return;
		}
		void library
			.serverSettings()
			.then((settings) => {
				roots = settings.settings['library.roots'].value;
				root = roots[0] ?? '';
			})
			.catch(() => {
				roots = [];
			});
	});

	async function save(fileName: string, replace: boolean): Promise<void> {
		busy = true;
		trouble = '';
		try {
			saved = await library.savePlaylist(playlist.id, {
				root,
				folder,
				name: fileName,
				...(replace ? { replace } : {})
			});
			step = 'saved';
		} catch (error) {
			if (error instanceof PlaylistExistsError) {
				exists = error.exists;
				step = 'confirm';
				return;
			}
			const status =
				typeof error === 'object' && error !== null && 'status' in error ? error.status : 0;
			trouble =
				status === 422
					? 'That name or folder won’t do: a name has no slashes, and the folder is in the library.'
					: status === 409
						? 'The library is busy, or tag writes are off. Try again in a moment.'
						: 'It couldn’t be saved. Try again.';
		} finally {
			busy = false;
		}
	}

	const savedText = $derived(saved === null ? null : savedWords(saved, root));

	// Buds from the ⋯'s Export… in place of the page's buttons; Save in the library's steps are the
	// same box reshaped; it gives way where it stands, and the buttons are laid again in front.
	const surface = new SurfaceSlot();
	reshapeOn(
		surface,
		() => 1,
		() => (canSave ? step : '')
	);
</script>

<div class="export" role="group" aria-label="Export the playlist" {@attach surface.attach}>
	<div class="head">
		<span class="what">{exportHead(playlist.songCount)}</span>
		<span class="snapshot">A SNAPSHOT: THE FILE DOESN’T FOLLOW LATER CHANGES</span>
		<span class="grow"></span>
		<button type="button" class="leave" onclick={onleave}>Leave it</button>
	</div>
	<div class={['cards', canSave && 'two']}>
		<div class="card">
			<span class="title">Download</span>
			<span class="body"
				>For another player or a backup. Paths start at the library folder, so it plays when you put
				it at the top of a copy of your music.</span
			>
			<span class="mono">{downloadLine(playlist.name, playlist.songCount)}</span>
			<!-- eslint-disable svelte/no-navigation-without-resolve -- The API's file, which the browser saves: not a route. -->
			<a
				class="download"
				href={library.playlistFileUrl(playlist.id)}
				download={exportFileName(playlist.name)}
				><svg
					width="15"
					height="15"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="2.2"
					stroke-linecap="round"
					stroke-linejoin="round"
					aria-hidden="true"><path d="M12 4v11M7 10l5 5 5-5M5 20h14"></path></svg
				>Download</a
			>
			<!-- eslint-enable svelte/no-navigation-without-resolve -->
		</div>
		{#if canSave}
			{#if step === 'choose'}
				<form
					class="card"
					aria-label="Save in the library"
					onsubmit={(event) => {
						event.preventDefault();
						void save(name, false);
					}}
				>
					<span class="title-row"
						><span class="title">Save in the library</span><span class="mono small"
							>ADMIN · TAG WRITES ON</span
						></span
					>
					<span class="where"
						><span class="mono small">IN</span><span class="folder"
							>{root === '' ? 'Playlists' : folderLabel(root, folder)}</span
						><button
							type="button"
							class="change"
							disabled={roots.length === 0}
							onclick={() => {
								picking = true;
							}}>Change folder</button
						></span
					>
					<label class="as"
						><span class="mono small">AS</span><input
							bind:value={name}
							aria-label="File name"
							autocomplete="off"
							spellcheck="false"
						/></label
					>
					<span class="note">Everyone here will see it under From files once it’s read.</span>
					{#if trouble !== ''}<span class="trouble" role="alert">{trouble}</span>{/if}
					<button type="submit" class="outline" disabled={busy || root === '' || name.trim() === ''}
						>Save here</button
					>
				</form>
			{:else if step === 'confirm' && exists !== null}
				<div class="card confirm" role="alertdialog" aria-label="The file is there already">
					<span class="title">{exists.name} is already there.</span>
					<span class="body"
						>{#each existsWords(exists, now) as part, i (i)}{#if part.bold}<b>{part.text}</b
								>{:else}{part.text}{/if}{/each}</span
					>
					<span class="choices">
						<button
							type="button"
							class="fox"
							disabled={busy}
							onclick={() => {
								if (exists !== null) void save(exists.name, true);
							}}>Replace it</button
						>
						<button
							type="button"
							class="outline"
							disabled={busy}
							onclick={() => {
								if (exists !== null) void save(exists.freeName, false);
							}}>{saveAsWords(exists.freeName)}</button
						>
						<button
							type="button"
							class="quiet"
							onclick={() => {
								step = 'choose';
								exists = null;
							}}>Keep it</button
						>
					</span>
				</div>
			{:else if step === 'saved' && savedText !== null}
				<div class="card saved" role="status">
					<span class="title saved-title"
						><svg
							width="18"
							height="18"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							stroke-width="2.6"
							stroke-linecap="round"
							stroke-linejoin="round"
							aria-hidden="true"><path d="M5 12.5l4.5 4.5L19 7"></path></svg
						>{savedText.head}</span
					>
					<span class="body">{savedText.body}</span>
					<button type="button" class="wash" onclick={() => void goto(resolve('/playlists'))}
						>See it in Playlists</button
					>
				</div>
			{/if}
		{/if}
	</div>
</div>

{#if canSave}
	<FolderPicker
		open={picking}
		within={roots}
		title="Save it in"
		choose="Save in"
		onadd={(path) => {
			const at = underRoot(path, roots);
			if (at !== null) {
				root = at.root;
				folder = at.folder;
			}
			picking = false;
		}}
		onclose={() => {
			picking = false;
		}}
	/>
{/if}

<style>
	.export {
		margin-top: 6px;
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	.head {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 4px 12px;
	}

	.what,
	.snapshot,
	.mono {
		font-family: var(--font-mono);
		letter-spacing: 0.06em;
	}

	.what {
		font-size: 12px;
		font-weight: 700;
		letter-spacing: 0.08em;
		color: var(--fox-text);
	}

	.snapshot {
		font-size: 11px;
		color: var(--text-meta);
	}

	.grow {
		flex: 1;
	}

	button,
	.download {
		height: 44px;
		box-sizing: border-box;
		display: inline-flex;
		align-items: center;
		gap: 8px;
		border: 0;
		border-radius: var(--radius-pill);
		color: var(--text);
		font: 600 14px var(--font-sans);
		text-decoration: none;
	}

	.leave,
	.wash {
		height: 36px;
		padding: 0 14px;
		background: var(--hover);
		font-size: 13px;
	}

	.wash {
		align-self: flex-start;
		height: 40px;
		padding: 0 16px;
	}

	.cards {
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		gap: 14px;
	}

	.cards.two {
		grid-template-columns: repeat(2, minmax(0, 1fr));
	}

	.card {
		display: flex;
		flex-direction: column;
		gap: 10px;
		padding: 18px 20px;
		border-radius: var(--radius-20);
		background: var(--card);
		border: 1px solid var(--edge);
	}

	.card.confirm {
		border: 2px solid var(--fox-line);
	}

	.card.saved {
		background: color-mix(in srgb, var(--fox) 12%, transparent);
		border-color: color-mix(in srgb, var(--fox-line) 55%, transparent);
	}

	.title {
		font-size: var(--type-title);
		font-weight: 700;
	}

	.title-row {
		display: flex;
		align-items: baseline;
		gap: 10px;
	}

	.saved-title {
		display: flex;
		align-items: center;
		gap: 8px;
	}

	.saved-title svg {
		color: var(--fox-line);
	}

	.body {
		font-size: 13px;
		line-height: 1.45;
		color: var(--text-secondary);
	}

	.mono {
		font-size: 11px;
		color: var(--text-meta);
	}

	.small {
		font-size: 10px;
		letter-spacing: 0.08em;
	}

	.download,
	.fox {
		padding: 0 22px;
		background: var(--fox);
		color: var(--fox-ink);
		font-weight: 700;
	}

	.download {
		align-self: flex-start;
		margin-top: 4px;
	}

	.where {
		display: flex;
		align-items: center;
		gap: 8px;
	}

	.folder {
		font-family: var(--font-mono);
		font-size: 13px;
	}

	.change {
		height: 30px;
		padding: 0 10px;
		background: none;
		color: var(--fox-text);
		font-size: 13px;
		text-decoration: underline;
		text-underline-offset: 3px;
	}

	.as {
		display: flex;
		align-items: center;
		gap: 8px;
	}

	input {
		flex: 1;
		min-width: 0;
		height: 36px;
		box-sizing: border-box;
		padding: 0 10px;
		border: 0;
		border-bottom: 2px solid var(--fox-line);
		border-radius: 8px 8px 0 0;
		background: var(--hover);
		outline: none;
		color: var(--text);
		font: 600 14px var(--font-sans);
	}

	.note {
		font-size: 12px;
		line-height: 1.45;
		color: var(--text-meta);
	}

	.trouble {
		font-size: 13px;
		color: var(--danger);
	}

	.outline {
		align-self: flex-start;
		padding: 0 20px;
		background: none;
		box-shadow: inset 0 0 0 1.5px var(--edge-strong);
		font-weight: 700;
	}

	.choices {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
		margin-top: 4px;
	}

	.choices .outline {
		font-weight: 600;
	}

	.quiet {
		padding: 0 16px;
		background: none;
		color: var(--text-secondary);
	}

	button:hover:not(:disabled) {
		filter: brightness(1.08);
	}

	button:disabled {
		opacity: 0.5;
	}
</style>
