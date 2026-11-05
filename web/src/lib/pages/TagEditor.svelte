<script lang="ts">
	import { onMount, tick, untrack } from 'svelte';
	import { library } from '#lib/library/library.svelte.ts';
	import type { Album } from '#lib/library/source.ts';
	import {
		advanced,
		draftOf,
		fieldId,
		problemsIn,
		rebased,
		reportOf,
		save,
		stepsOf,
		yearField,
		type Report
	} from '#lib/library/tags.ts';
	import GenreField from './GenreField.svelte';
	import NumberField from './NumberField.svelte';

	interface Props {
		album: Album;
		/** Leaves the editor, saying whether anything it held was saved. */
		close: (saved: boolean) => void;
	}

	let { album, close }: Props = $props();

	/** What the fields are measured against: the album as opened, and whatever has landed since. */
	let base = $state.raw(untrack(() => draftOf(album)));
	let draft = $state(untrack(() => draftOf(album)));
	/** Problems wait for a save: a year half typed isn't wrong yet. */
	let attempted = $state(false);
	let saving = $state(false);
	let report = $state.raw<Report | null>(null);
	let heading: HTMLElement | undefined = $state();
	let reportBox: HTMLElement | undefined = $state();

	const was = $derived(new Map(base.tracks.map((row) => [row.id, row])));
	const tracks = $derived(new Map(album.tracks.map((track) => [track.id, track])));
	const problems = $derived(attempted ? problemsIn(base, draft) : new Map<string, string>());
	const mixedGenres = $derived(
		album.tracks.some(({ genres }) => genres.length !== album.genres.length)
	);

	onMount(() => {
		heading?.focus();
	});

	function titleOf(id: string): string {
		return was.get(id)?.title ?? '';
	}

	async function submit(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		if (saving || report?.retry === false) {
			return;
		}
		attempted = true;
		const [first] = problemsIn(base, draft).keys();
		if (first !== undefined) {
			await tick();
			document.getElementById(first)?.focus();
			return;
		}
		const steps = stepsOf(base, draft);
		if (steps.length === 0) {
			close(false);
			return;
		}
		saving = true;
		report = null;
		const saved = await save(library, album.id, steps);
		base = advanced(base, saved.done);
		if (saved.stop === null) {
			close(true);
			return;
		}
		report = reportOf(saved, titleOf);
		if (saved.stop.kind === 'stale') {
			const fresh = draftOf(library.album(album.id) ?? album);
			draft = rebased(base, draft, fresh);
			base = fresh;
		}
		saving = false;
		await tick();
		reportBox?.focus();
	}
</script>

{#snippet problem(field: string)}
	{@const text = problems.get(field)}
	{#if text !== undefined}<p class="problem" id={`${field}-problem`}>{text}</p>{/if}
{/snippet}

<form
	class="editor"
	aria-labelledby="edit-tags"
	novalidate
	onsubmit={(event) => void submit(event)}
>
	<h2 id="edit-tags" tabindex="-1" bind:this={heading}>Edit tags</h2>

	<fieldset class="album" disabled={saving}>
		<legend class="visually-hidden">Album</legend>
		<NumberField
			id={yearField}
			label="Year"
			was={base.year}
			invalid={problems.has(yearField)}
			bind:value={draft.year}
		/>
		<GenreField id="genres" bind:genres={draft.genres} />
		{@render problem(yearField)}
		{#if mixedGenres}
			<p class="note">Tracks here differ. A change of genres gives every track this whole list.</p>
		{/if}
	</fieldset>

	<fieldset class="tracks" disabled={saving}>
		<legend class="visually-hidden">Tracks</legend>
		<div class="row head" aria-hidden="true">
			<span>Disc</span>
			<span>Track</span>
			<span>Title</span>
		</div>
		<ul>
			{#each draft.tracks as row (row.id)}
				{@const old = was.get(row.id)}
				{@const track = tracks.get(row.id)}
				{@const name = old?.title ?? row.title}
				{@const title = fieldId(row.id, 'title')}
				<li class="row">
					<NumberField
						id={fieldId(row.id, 'discNo')}
						label="Disc"
						whose={name}
						columned
						was={old?.discNo ?? ''}
						total={track?.discTotal}
						invalid={problems.has(fieldId(row.id, 'discNo'))}
						bind:value={row.discNo}
					/>
					<NumberField
						id={fieldId(row.id, 'trackNo')}
						label="Track"
						whose={name}
						columned
						was={old?.trackNo ?? ''}
						total={track?.trackTotal}
						invalid={problems.has(fieldId(row.id, 'trackNo'))}
						bind:value={row.trackNo}
					/>
					<div class="title">
						<label class="visually-hidden" for={title}>Title of {name}</label>
						<input
							id={title}
							type="text"
							autocomplete="off"
							aria-invalid={problems.has(title)}
							aria-describedby={problems.has(title) ? `${title}-problem` : undefined}
							bind:value={row.title}
						/>
					</div>
					{@render problem(fieldId(row.id, 'discNo'))}
					{@render problem(fieldId(row.id, 'trackNo'))}
					{@render problem(title)}
				</li>
			{/each}
		</ul>
	</fieldset>

	<p class="note">Years and numbers can be changed, not removed. Totals stay as they are.</p>

	{#if report !== null}
		<div class="report" role="alert" tabindex="-1" bind:this={reportBox}>
			<p class="message">{report.message}</p>
			{#if report.detail !== undefined}<p class="detail">{report.detail}</p>{/if}
			{#if report.changed.length > 0}
				<dl>
					<div>
						<dt>Saved</dt>
						{#each report.changed as item, index (index)}<dd>{item}</dd>{/each}
					</div>
					<div>
						<dt>{report.unchangedHeading}</dt>
						{#each report.unchanged as item, index (index)}<dd>{item}</dd>{/each}
					</div>
				</dl>
			{/if}
		</div>
	{/if}

	<div class="actions">
		<button type="submit" class="save" disabled={saving || report?.retry === false}>
			{saving ? 'Saving…' : 'Save'}
		</button>
		<button
			type="button"
			class="cancel"
			disabled={saving}
			onclick={() => {
				close(false);
			}}>Cancel</button
		>
	</div>
</form>

<style>
	.editor {
		display: flex;
		flex-direction: column;
		gap: 24px;
		padding: 16px;
		animation: arrive var(--morph-fast) both;
	}

	@keyframes arrive {
		from {
			opacity: 0;
		}
	}

	/* Whatever takes focus must clear the nav and the island, which float over the page. */
	.editor :global(:is(input, button)),
	.report {
		scroll-margin: 96px 0 160px;
	}

	h2 {
		font-size: var(--type-title);
		font-weight: 700;
		letter-spacing: var(--tracking-title);
		outline: none;
	}

	fieldset {
		min-width: 0;
		margin: 0;
		padding: 0;
		border: 0;
	}

	fieldset:disabled {
		opacity: 0.6;
	}

	.album {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr);
		gap: 12px 24px;
	}

	.album .problem,
	.album .note {
		grid-column: 1 / -1;
	}

	.tracks ul {
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.row {
		display: grid;
		grid-template-columns: 136px 136px minmax(0, 1fr);
		gap: 6px 16px;
		align-items: center;
		padding: 8px 0;
		border-top: 1px solid var(--edge-card);
	}

	.head {
		padding: 0 0 8px;
		border-top: 0;
		font-size: 13px;
		font-weight: 600;
		color: var(--text-meta);
	}

	.title input {
		width: 100%;
		height: var(--target);
		padding: 0 18px;
		border: 1px solid var(--edge-strong);
		border-radius: var(--radius-pill);
		background: rgb(10 9 8 / 30%);
		color: var(--text);
		font: inherit;
		font-size: 16px;
		font-weight: 500;
	}

	.title input[aria-invalid='true'] {
		border-color: var(--fox);
	}

	.problem {
		grid-column: 1 / -1;
		font-size: 14px;
		color: var(--fox);
	}

	.note {
		font-size: 14px;
		color: var(--text-meta);
	}

	.report {
		display: flex;
		flex-direction: column;
		gap: 12px;
		padding: 16px 18px;
		border-radius: var(--radius-20);
		background: rgb(255 255 255 / 5%);
		outline: none;
	}

	.message {
		font-size: var(--type-body);
		font-weight: 600;
		color: var(--fox);
	}

	.detail {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		color: var(--text-meta);
		overflow-wrap: anywhere;
	}

	dl {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
		gap: 12px 24px;
		margin: 0;
	}

	dt {
		font-size: 13px;
		font-weight: 600;
		color: var(--text-meta);
	}

	dd {
		margin: 4px 0 0;
		font-size: 14px;
		overflow-wrap: anywhere;
	}

	.actions {
		display: flex;
		gap: 12px;
	}

	.actions button {
		height: var(--target);
		padding: 0 22px;
		border-radius: var(--radius-pill);
		font-size: 14px;
		font-weight: 600;
	}

	.save {
		border: 0;
		background: var(--accent);
		color: var(--on-accent);
	}

	.cancel {
		border: 1px solid var(--edge-strong);
		background: none;
	}

	.actions button:disabled {
		opacity: 0.5;
		cursor: default;
	}

	@media (max-width: 820px) {
		.editor {
			padding: 12px 8px;
		}

		.album {
			grid-template-columns: minmax(0, 1fr);
		}

		.head {
			display: none;
		}

		.row {
			grid-template-columns: repeat(2, minmax(0, 1fr));
			gap: 10px 12px;
			padding: 14px 0;
		}

		.title {
			grid-column: 1 / -1;
		}
	}
</style>
