<script lang="ts">
	import { yearField, type AlbumWide } from '#lib/library/tags.ts';
	import ChipField from './ChipField.svelte';
	import type { TagSession } from './tagEdits.svelte.ts';

	interface Props {
		session: TagSession;
		wide: AlbumWide;
	}

	let { session, wide }: Props = $props();

	const year = $derived(session.draft.year ?? wide.year);
	const yearChanged = $derived(year.trim() !== wide.year);
	const genres = $derived(session.draft.genres ?? wide.genres);
	const genresChanged = $derived(
		genres.length !== wide.genres.length ||
			genres.some((genre, index) => genre !== wide.genres[index])
	);
	const problem = $derived(session.problem(yearField));
	const notes = $derived(
		[
			wide.yearsDiffer && yearChanged ? 'Every track takes this year.' : '',
			wide.genresDiffer && genresChanged ? 'Every track takes all of these genres.' : ''
		].filter((note) => note !== '')
	);
</script>

<div class="fields">
	<label class="box year" class:changed={yearChanged} class:invalid={problem !== undefined}>
		<span class="name">YEAR</span>
		<input
			id={yearField}
			type="text"
			inputmode="numeric"
			autocomplete="off"
			aria-label="Year"
			aria-invalid={problem !== undefined}
			aria-describedby={problem === undefined ? undefined : `${yearField}-problem`}
			placeholder={wide.yearsDiffer ? 'Mixed' : 'Empty'}
			bind:value={
				() => year,
				(value) => {
					session.setYear(value);
				}
			}
		/>
		{#if yearChanged && wide.year !== ''}
			<span class="was"><span class="visually-hidden">was </span>{wide.year}</span>
		{/if}
	</label>
	<div class="box genres" class:changed={genresChanged}>
		<span class="name">GENRES</span>
		<ChipField
			id="genres"
			label="Add a genre"
			commas
			bind:values={
				() => genres,
				(values) => {
					session.setGenres(values);
				}
			}
		/>
	</div>
</div>
{#if problem !== undefined}
	<p class="problem" id={`${yearField}-problem`}>{problem}</p>
{:else if notes.length > 0}
	<p class="note">{notes.join(' ')}</p>
{/if}

<style>
	.fields {
		display: flex;
		align-items: center;
		gap: 12px;
		min-width: 0;
		max-width: calc(100cqw - 2 * var(--hero-pad) - var(--hero-cover) - 32px);
	}

	.box {
		display: flex;
		align-items: center;
		gap: 10px;
		height: 48px;
		padding: 0 16px;
		border-radius: var(--radius-16);
		background: color-mix(in srgb, var(--ground) 42%, transparent);
		transition: box-shadow var(--morph-fast) var(--curve-morph);
	}

	.genres {
		flex: 0 1 auto;
		min-width: 0;
		gap: 8px;
		padding: 0 8px 0 16px;
	}

	.year {
		flex: none;
	}

	.changed {
		box-shadow: inset 0 0 0 1px var(--fox-text);
	}

	.invalid {
		box-shadow: inset 0 0 0 1px var(--problem-edge);
	}

	.name {
		flex: none;
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.08em;
		color: var(--wash);
	}

	input {
		width: 56px;
		padding: 0;
		border: 0;
		outline: none;
		background: transparent;
		color: var(--text);
		font: inherit;
		font-size: 17px;
		font-weight: 700;
	}

	input::placeholder {
		font-weight: 500;
		color: var(--wash);
	}

	.was {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		color: var(--wash);
		text-decoration: line-through;
	}

	/* Under the fields, in what's left of the stage: they mustn't move for it. */
	.note,
	.problem {
		position: absolute;
		top: calc(100% + 8px);
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		color: var(--wash);
	}

	.problem {
		color: var(--problem);
	}

	@media (max-width: 820px) {
		.fields {
			flex-direction: column;
			align-items: stretch;
			gap: 8px;
			max-width: none;
		}

		.name {
			width: 64px;
			font-size: var(--type-mono);
		}

		.box {
			padding: 0 16px;
		}

		.year input {
			flex: 1;
			min-width: 0;
			width: auto;
		}

		.genres {
			flex: none;
		}

		.note,
		.problem {
			position: static;
		}
	}
</style>
