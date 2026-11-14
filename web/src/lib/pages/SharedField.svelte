<script lang="ts">
	import { isSharedChanged, sharedFieldId, shownOf, type SharedSpec } from '#lib/library/edit.ts';
	import ChipField from './ChipField.svelte';
	import type { EditSession } from './tagEditor.svelte.ts';

	interface Props {
		spec: SharedSpec;
		session: EditSession;
	}

	let { spec, session }: Props = $props();

	const id = $derived(sharedFieldId(spec.key));
	const shown = $derived(shownOf(session.tickedTracks, session.draft, spec.key));
	const changed = $derived(isSharedChanged(session.tickedTracks, session.draft, spec.key));
	const problem = $derived(session.problem(id));
	/** "Mixed" says the songs differ: left alone, each keeps its own. */
	const hint = $derived(shown.mixed ? 'Mixed' : 'Empty');
</script>

<div class="field" class:changed class:invalid={problem !== undefined} style:--span={spec.span}>
	<label class="label" for={id}>{spec.label}</label>
	{#if spec.multi}
		<ChipField
			{id}
			label={spec.label}
			placeholder={shown.values.length === 0 ? hint : 'Add'}
			commas={spec.key === 'genres'}
			bind:values={
				() => shown.values,
				(next) => {
					session.setShared(spec.key, next);
				}
			}
		/>
	{:else}
		<input
			{id}
			type="text"
			inputmode={spec.kind === 'text' ? undefined : 'numeric'}
			autocomplete="off"
			placeholder={hint}
			aria-invalid={problem !== undefined}
			aria-describedby={problem === undefined ? undefined : `${id}-problem`}
			bind:value={
				() => shown.values.at(0) ?? '',
				(value) => {
					session.setShared(spec.key, [value]);
				}
			}
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

	.label {
		display: block;
		font-size: var(--type-mono);
		color: var(--text-meta);
	}

	input {
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

	.field :global(input::placeholder) {
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
