<script lang="ts">
	import type { AlbumTags } from '#lib/library/source.ts';
	import {
		fieldId,
		isChanged,
		isMulti,
		valueOf,
		wasOf,
		type FieldSpec
	} from '#lib/library/tags.ts';
	import ChipField from './ChipField.svelte';
	import type { TagSession } from './tagEdits.svelte.ts';

	interface Props {
		spec: FieldSpec;
		trackId: string;
		tags: AlbumTags;
		session: TagSession;
	}

	let { spec, trackId, tags, session }: Props = $props();

	const { field } = $derived(spec);
	const id = $derived(fieldId(trackId, field));
	const values = $derived(valueOf(tags, session.draft, trackId, field));
	const was = $derived(wasOf(tags, trackId, field));
	const changed = $derived(isChanged(tags, session.draft, trackId, field));
	const problem = $derived(session.problem(id));
	const multi = $derived(isMulti(field));
	const shown = $derived(spec.locked && field === 'compilation' ? [compiled(values)] : values);
	/** A value with lines in it would lose them in a one-line input. */
	const lines = $derived(values.at(0)?.includes('\n') ?? false);

	/** Reads a compilation flag as a person would. */
	function compiled([value]: readonly string[]): string {
		return value === undefined || value === '0' ? 'No' : 'Yes';
	}

	function set(value: string): void {
		session.setField(trackId, field, [value]);
	}
</script>

<div
	class="field"
	class:changed
	class:invalid={problem !== undefined}
	class:locked={spec.locked}
	style:--span={spec.span}
>
	<div class="head">
		{#if spec.locked}
			<span class="label">{spec.label}</span>
		{:else}
			<label class="label" for={id}>
				{spec.label}{#if spec.name !== undefined}<span class="visually-hidden">
						({spec.name})</span
					>{/if}
			</label>
		{/if}
		{#if changed && was.length > 0}
			<span class="was"><span class="visually-hidden">was </span>{was.join(', ')}</span>
		{:else if !multi && was.length > 1}
			<span class="more">1 of {was.length} values</span>
		{/if}
	</div>
	{#if spec.locked}
		{#if multi && shown.length > 0}
			<ul class="fixed">
				{#each shown as value, index (index)}<li>{value}</li>{/each}
			</ul>
		{:else}
			<p class="fixed" class:empty={shown.length === 0}>{shown.at(0) ?? 'Empty'}</p>
		{/if}
	{:else if multi}
		<ChipField
			{id}
			label={spec.name ?? spec.label}
			placeholder="Empty"
			commas={field === 'genres'}
			bind:values={
				() => values,
				(next) => {
					session.setField(trackId, field, next);
				}
			}
		/>
	{:else if lines}
		<textarea
			{id}
			rows="3"
			aria-invalid={problem !== undefined}
			bind:value={() => values.at(0) ?? '', set}></textarea>
	{:else}
		<input
			{id}
			type="text"
			inputmode={spec.kind === 'number' ? 'numeric' : undefined}
			autocomplete="off"
			placeholder="Empty"
			aria-invalid={problem !== undefined}
			aria-describedby={problem === undefined ? undefined : `${id}-problem`}
			bind:value={() => values.at(0) ?? '', set}
		/>
	{/if}
	{#if problem !== undefined}<p class="problem" id={`${id}-problem`}>{problem}</p>{/if}
</div>

<style>
	.field {
		grid-column: span var(--span);
		min-width: 0;
		padding-bottom: 4px;
		border-bottom: 1px solid var(--edge-field);
		transition: border-color var(--morph-fast) var(--curve-morph);
	}

	.field:focus-within {
		border-bottom-color: var(--text);
	}

	.changed {
		border-bottom-color: var(--fox-text);
	}

	.invalid {
		border-bottom-color: var(--problem-edge);
	}

	/* Read, not edited: nothing here says it can be typed in. */
	.locked {
		border-bottom-color: transparent;
	}

	.head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 12px;
		min-width: 0;
	}

	.label {
		flex: none;
		display: block;
		font-size: var(--type-mono);
		color: var(--text-meta);
	}

	.was,
	.more {
		min-width: 0;
		overflow: hidden;
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--text-meta);
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.was {
		text-decoration: line-through;
	}

	input,
	textarea {
		display: block;
		width: 100%;
		height: 32px;
		padding: 0;
		border: 0;
		outline: none;
		background: transparent;
		color: var(--text);
		font: inherit;
		font-size: var(--type-body);
		font-weight: 600;
	}

	textarea {
		height: auto;
		padding-top: 6px;
		line-height: 1.4;
		resize: none;
	}

	input::placeholder {
		font-weight: 500;
		color: var(--text-label);
	}

	.fixed {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0 12px;
		min-height: 32px;
		margin: 0;
		padding: 0;
		list-style: none;
		font-size: var(--type-body);
		font-weight: 600;
		color: var(--text-secondary);
	}

	.fixed.empty {
		font-weight: 500;
		color: var(--text-label);
	}

	.problem {
		padding-top: 2px;
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--problem);
	}
</style>
