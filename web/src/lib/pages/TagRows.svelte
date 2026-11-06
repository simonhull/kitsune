<script lang="ts">
	import { clock } from '#lib/library/format.ts';
	import type { Album, AlbumTags, TagField } from '#lib/library/source.ts';
	import { fieldId, isChanged, rowNote, valueOf } from '#lib/library/tags.ts';
	import NumberField from './NumberField.svelte';
	import type { TagSession } from './tagEdits.svelte.ts';

	interface Props {
		album: Album;
		tags: AlbumTags;
		session: TagSession;
	}

	let { album, tags, session }: Props = $props();

	const lengths = $derived(new Map(album.tracks.map((track) => [track.id, track.durationMs])));

	function first(id: string, field: TagField): string {
		return valueOf(tags, session.draft, id, field).at(0) ?? '';
	}

	function left({ target }: FocusEvent): void {
		if (target instanceof HTMLElement && target.id !== '') {
			session.leftField(target.id);
		}
	}
</script>

{#snippet number(id: string, whose: string, field: TagField, label: string, quiet: boolean)}
	<NumberField
		id={fieldId(id, field)}
		{label}
		{whose}
		{quiet}
		changed={isChanged(tags, session.draft, id, field)}
		invalid={session.problem(fieldId(id, field)) !== undefined}
		bind:value={
			() => first(id, field),
			(value) => {
				session.setField(id, field, [value]);
			}
		}
	/>
{/snippet}

<ul class="rows" onfocusout={left}>
	{#each tags.tracks as track (track.id)}
		{@const title = fieldId(track.id, 'title')}
		{@const name = track.fields.title?.at(0) ?? track.file}
		{@const note = rowNote(tags, session.draft, track.id)}
		{@const problems = (['disc', 'track', 'title'] as const).flatMap(
			(field) => session.problem(fieldId(track.id, field)) ?? []
		)}
		{@const invalid = (['disc', 'track', 'title'] as const).some((field) =>
			session.hasProblem(fieldId(track.id, field))
		)}
		{@const changed = note.old !== '' || note.text !== ''}
		<li class="row" class:changed class:invalid>
			<span class="dot" aria-hidden="true"></span>
			{@render number(track.id, name, 'disc', 'DISC', true)}
			{@render number(track.id, name, 'track', 'TRACK', false)}
			<input
				id={title}
				class="title"
				type="text"
				autocomplete="off"
				aria-label={`Title of ${name}`}
				aria-invalid={session.problem(title) !== undefined}
				aria-describedby={session.problem(title) === undefined ? undefined : `${title}-problem`}
				placeholder={track.fields.title === undefined ? 'Empty' : undefined}
				bind:value={
					() => first(track.id, 'title'),
					(value) => {
						session.setField(track.id, 'title', [value]);
					}
				}
			/>
			{#if problems.length > 0}
				<span class="note problem" id={`${title}-problem`}>{problems.join(' ')}</span>
			{:else if changed}
				<span class="note">
					{#if note.old !== ''}<span class="old"
							><span class="visually-hidden">was </span>{note.old}</span
						>{/if}
					{note.text}
				</span>
			{:else}
				<span class="note blank"></span>
			{/if}
			<span class="length">{clock((lengths.get(track.id) ?? 0) / 1000)}</span>
		</li>
	{/each}
</ul>

<style>
	.rows {
		margin: 0;
		padding: 0;
		list-style: none;
		animation: arrive var(--morph-fast) both;
	}

	@keyframes arrive {
		from {
			opacity: 0;
		}
	}

	.row {
		display: flex;
		align-items: center;
		gap: 16px;
		height: 56px;
		border-bottom: 1px solid var(--edge-card);
	}

	.dot {
		flex: none;
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: transparent;
		transition: background var(--morph-fast) var(--curve-morph);
	}

	.changed .dot {
		background: var(--fox);
	}

	.invalid .dot {
		background: var(--problem-edge);
	}

	.title {
		flex: 1;
		min-width: 0;
		height: 40px;
		padding: 0;
		border: 0;
		border-bottom: 1px solid rgb(255 255 255 / 14%);
		outline: none;
		background: transparent;
		color: var(--text);
		font: inherit;
		font-size: 16px;
		font-weight: 600;
	}

	.title:focus {
		border-bottom-color: var(--text);
	}

	.title::placeholder {
		font-weight: 500;
		color: var(--text-label);
	}

	.changed .title {
		border-bottom-color: var(--fox);
	}

	.invalid .title {
		border-bottom-color: var(--problem-edge);
	}

	.note {
		flex: none;
		width: 220px;
		overflow: hidden;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		color: var(--text-meta);
		text-align: right;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.old {
		text-decoration: line-through;
	}

	.problem {
		color: var(--problem);
	}

	.length {
		flex: none;
		width: 48px;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		font-variant-numeric: tabular-nums;
		color: var(--text-meta);
		text-align: right;
	}

	/* Whatever takes focus must clear the bar and the island, which float over the page. */
	.rows :global(input) {
		scroll-margin: 96px 0 200px;
	}

	@media (max-width: 820px) {
		.row {
			display: grid;
			grid-template-columns: auto auto minmax(0, 1fr);
			grid-template-areas:
				'title title title'
				'disc track length'
				'note note note';
			gap: 2px 10px;
			height: auto;
			padding: 10px 0;
		}

		.dot {
			display: none;
		}

		.row > :global(:nth-child(2)) {
			grid-area: disc;
		}

		.row > :global(:nth-child(3)) {
			grid-area: track;
		}

		.title {
			grid-area: title;
			height: 44px;
		}

		.note {
			grid-area: note;
			width: auto;
			text-align: left;
		}

		/* An empty note takes no line: the rows keep the board's height until there's news. */
		.blank {
			display: none;
		}

		.length {
			grid-area: length;
			width: auto;
			align-self: center;
		}
	}
</style>
