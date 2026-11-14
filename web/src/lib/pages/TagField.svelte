<script lang="ts">
	import { isChanged, valueOf, wasOf, type EditTrack } from '#lib/library/edit.ts';
	import { fieldId, isMulti, type FieldSpec } from '#lib/library/tags.ts';
	import ChipField from './ChipField.svelte';
	import type { EditSession } from './tagEditor.svelte.ts';

	interface Props {
		spec: FieldSpec;
		track: EditTrack;
		session: EditSession;
	}

	let { spec, track, session }: Props = $props();

	const { field } = $derived(spec);
	/** The problem's key. The title's row holds that ID, so the open song's own title takes another. */
	const key = $derived(fieldId(track.id, field));
	const id = $derived(field === 'title' ? `open-${key}` : key);
	const values = $derived(valueOf(track, session.draft, session.ticked, field));
	const was = $derived(wasOf(track, field));
	const changed = $derived(isChanged(track, session.draft, session.ticked, field));
	const problem = $derived(session.problem(key));
	const multi = $derived(isMulti(field));
	const shown = $derived(spec.locked && field === 'compilation' ? [compiled(values)] : values);
	/** A value with lines in it would lose them in a one-line input. */
	const lines = $derived(values.at(0)?.includes('\n') ?? false);

	/** Reads a compilation flag as a person would. */
	function compiled([value]: readonly string[]): string {
		return value === undefined || value === '0' ? 'No' : 'Yes';
	}

	function set(value: string): void {
		session.setField(track.id, field, [value]);
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
					session.setField(track.id, field, next);
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
		padding-bottom: 2px;
		border-bottom: 1px solid var(--edge-field);
		transition: border-color var(--morph-fast) var(--curve-morph);
	}

	.field:focus-within {
		border-bottom-color: var(--text);
	}

	.changed {
		border-bottom-color: var(--fox-line);
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

	.field :global(input::placeholder) {
		font-style: italic;
		font-weight: 400;
		color: var(--text-meta);
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
		overflow-wrap: anywhere;
	}

	.fixed.empty {
		font-style: italic;
		font-weight: 400;
		color: var(--text-meta);
	}

	.problem {
		padding-top: 2px;
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--problem);
	}
</style>
