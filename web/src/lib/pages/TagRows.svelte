<script lang="ts">
	import { tick } from 'svelte';
	import { clock } from '#lib/library/format.ts';
	import type { Album, AlbumTags, TagField } from '#lib/library/source.ts';
	import { fieldId, isChanged, isTrackEdited, rowNote, valueOf } from '#lib/library/tags.ts';
	import { reducedMotion } from '#lib/morph/journey.ts';
	import Icon from '#lib/shell/Icon.svelte';
	import NumberField from './NumberField.svelte';
	import TagPanel from './TagPanel.svelte';
	import type { TagSession } from './tagEdits.svelte.ts';

	interface Props {
		album: Album;
		tags: AlbumTags;
		session: TagSession;
	}

	let { album, tags, session }: Props = $props();

	/** How long the page takes to draw up around an open track. */
	const settleMs = 440;

	const open = $derived(session.open);
	const lengths = $derived(new Map(album.tracks.map((track) => [track.id, track.durationMs])));

	function first(id: string, field: TagField): string {
		return valueOf(tags, session.draft, id, field).at(0) ?? '';
	}

	/** Whether a track is opening from the album page, rather than from another track. */
	let arriving = $state(false);

	/** Opens a track, and brings it up if it would open below the fold. */
	function expand(id: string): void {
		arriving = open === null;
		session.expand(id);
		setTimeout(() => {
			const panel = document.querySelector('[data-open-track]');
			if (panel !== null && panel.getBoundingClientRect().top > innerHeight * 0.4) {
				panel.scrollIntoView({ behavior: reducedMotion() ? 'auto' : 'smooth', block: 'start' });
			}
		}, settleMs);
	}

	async function collapse(id: string): Promise<void> {
		session.expand(null);
		await tick();
		document.getElementById(`more-${id}`)?.focus();
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
	{#each tags.tracks as track, index (track.id)}
		{@const title = fieldId(track.id, 'title')}
		{@const name = track.fields.title?.at(0) ?? track.file}
		{@const note = rowNote(tags, session.draft, track.id)}
		{@const problems = (['disc', 'track', 'title'] as const).flatMap(
			(field) => session.problem(fieldId(track.id, field)) ?? []
		)}
		{@const invalid = (['disc', 'track', 'title'] as const).some((field) =>
			session.hasProblem(fieldId(track.id, field))
		)}
		{@const edited = isTrackEdited(tags, session.draft, track.id)}
		{@const noted = note.old !== '' || note.text !== ''}
		{#if open === track.id}
			<li data-open-track>
				<TagPanel
					{album}
					{tags}
					{track}
					{session}
					fly={arriving}
					close={() => {
						void collapse(track.id);
					}}
				/>
			</li>
		{:else if open !== null}
			<li>
				<button
					type="button"
					class="quiet"
					class:marked={edited}
					onclick={() => {
						expand(track.id);
					}}
				>
					<span class="dot" aria-hidden="true"></span>
					<span class="place">{String(index + 1).padStart(2, '0')}</span>
					<span class="name">{first(track.id, 'title') || track.file}</span>
					<span class="length">{clock((lengths.get(track.id) ?? 0) / 1000)}</span>
				</button>
			</li>
		{:else}
			<li class="row" class:changed={edited} class:invalid>
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
				{:else if noted}
					<span class="note">
						{#if note.old !== ''}<span class="old"
								><span class="visually-hidden">was </span>{note.old}</span
							>{/if}
						{note.text}
					</span>
				{:else if edited}
					<span class="note">other tags changed</span>
				{:else}
					<span class="note blank"></span>
				{/if}
				<span class="length">{clock((lengths.get(track.id) ?? 0) / 1000)}</span>
				<button
					type="button"
					id={`more-${track.id}`}
					class="more"
					aria-label={`Every tag of ${name}`}
					onclick={() => {
						expand(track.id);
					}}
				>
					<Icon name="chevron-down" size={16} />
				</button>
			</li>
		{/if}
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
		border-bottom: 1px solid var(--edge-field);
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
		border-bottom-color: var(--fox-text);
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

	.more {
		flex: none;
		display: flex;
		align-items: center;
		justify-content: center;
		width: var(--target);
		height: var(--target);
		margin: 0 -14px 0 -12px;
		border: 0;
		border-radius: 50%;
		background: none;
		color: var(--text-label);
	}

	.more:hover {
		color: var(--fox-text);
	}

	.quiet {
		position: relative;
		display: flex;
		align-items: center;
		gap: 16px;
		width: 100%;
		height: 52px;
		padding: 0;
		border: 0;
		border-bottom: 1px solid var(--edge-card);
		background: none;
		color: var(--text-meta);
		font-size: var(--type-body);
		text-align: left;
	}

	.quiet:hover {
		color: var(--text);
	}

	.quiet .dot {
		position: absolute;
		left: -14px;
	}

	.quiet.marked .dot {
		background: var(--fox);
	}

	.place {
		flex: none;
		width: 28px;
		font-family: var(--font-mono);
		font-size: 13px;
	}

	.name {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		font-weight: 600;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.quiet .length {
		color: inherit;
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

		.dot,
		.more {
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
