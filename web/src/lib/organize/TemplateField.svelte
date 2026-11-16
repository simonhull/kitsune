<!--
	A path template, drawn as the path it makes: fields as fox chips that can be taken out, folder
	breaks as thin dividers, brackets in the fox, the rest as typed. What's typed lands at the end;
	typing < opens the field list, Backspace on nothing takes the last piece off, and Text shows the
	template as plain text to edit anywhere in it. A fault is marked where the parser found it.
-->
<script lang="ts">
	import Icon from '#lib/shell/Icon.svelte';
	import { holds, partsOf, without } from './parts.ts';
	import { fieldNotes, fields } from './template.ts';

	interface Props {
		value: string;
		label: string;
		/** Where it goes wrong, as the parser (or the server) says. */
		fault: { at: number; reason: string } | null;
		/** The main field is the bigger, ringed one. */
		main?: boolean;
		onchange: (value: string) => void;
	}

	let { value, label, fault, main = false, onchange }: Props = $props();

	const id = $props.id();
	const parts = $derived(partsOf(value));
	const length = $derived(Array.from(value).length);
	/** What is being typed after a <, while the field list is open. */
	let pending = $state('');
	let asText = $state(false);
	let typing: HTMLInputElement | undefined = $state();
	let chosen = $state(0);

	const listed = $derived.by(() => {
		if (!pending.startsWith('<')) {
			return [];
		}
		const wanted = pending.slice(1).toLowerCase();
		return fields.filter((f) => f.toLowerCase().includes(wanted));
	});

	function put(next: string): void {
		onchange(next);
	}

	function typed(text: string): void {
		const lt = text.indexOf('<');
		if (lt < 0) {
			put(value + text);
			pending = '';
			return;
		}
		if (lt > 0) {
			put(value + text.slice(0, lt));
		}
		const rest = text.slice(lt);
		const gt = rest.indexOf('>');
		if (gt >= 0) {
			put(value + text.slice(0, lt) + rest.slice(0, gt + 1));
			pending = '';
			typed(rest.slice(gt + 1));
			return;
		}
		pending = rest;
		chosen = 0;
	}

	function pick(field: string): void {
		put(`${value}<${field}>`);
		pending = '';
		typing?.focus();
	}

	function key(event: KeyboardEvent): void {
		if (listed.length > 0) {
			if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
				event.preventDefault();
				const step = event.key === 'ArrowDown' ? 1 : -1;
				chosen = (chosen + step + listed.length) % listed.length;
				return;
			}
			if (event.key === 'Enter' || event.key === 'Tab') {
				const field = listed[chosen];
				if (field !== undefined) {
					event.preventDefault();
					pick(field);
				}
				return;
			}
		}
		if (event.key === 'Escape' && pending !== '') {
			event.preventDefault();
			event.stopPropagation();
			put(value + pending);
			pending = '';
			return;
		}
		if (event.key === 'Backspace' && pending === '') {
			const last = parts.at(-1);
			if (last !== undefined) {
				event.preventDefault();
				put(without(value, last));
			}
		}
	}
</script>

<div class="field" class:main class:faulty={fault !== null}>
	{#if asText}
		<input
			class="text"
			aria-label={label}
			spellcheck="false"
			autocomplete="off"
			{value}
			oninput={(event) => {
				put(event.currentTarget.value);
			}}
		/>
	{:else}
		<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
		<div
			class="chips"
			onclick={(event) => {
				if (event.target === event.currentTarget) {
					typing?.focus();
				}
			}}
		>
			{#each parts as part (part.start)}
				{@const at = fault !== null && holds(part, fault.at, length)}
				{#if part.kind === 'token'}
					<span class="chip" class:fault={at}
						><span class="chip-name">{part.text}</span><button
							type="button"
							class="drop"
							aria-label={`Take out ${part.text}`}
							onclick={() => {
								put(without(value, part));
								typing?.focus();
							}}><Icon name="close" size={11} /></button
						></span
					>
				{:else if part.kind === 'slash'}
					<span class="slash" class:fault={at} aria-label="a folder">/</span>
				{:else if part.kind === 'literal'}
					<span class="literal" class:fault={at}>{part.text}</span>
				{:else}
					<span class="bracket" class:fault={at}>{part.text}</span>
				{/if}
			{/each}
			<input
				bind:this={typing}
				class="typing"
				aria-label={label}
				aria-describedby={`${id}-hint`}
				aria-expanded={listed.length > 0}
				aria-controls={`${id}-list`}
				role="combobox"
				aria-autocomplete="list"
				spellcheck="false"
				autocomplete="off"
				value={pending}
				size={Math.max(2, Array.from(pending).length + 1)}
				oninput={(event) => {
					typed(event.currentTarget.value);
					event.currentTarget.value = pending;
				}}
				onkeydown={key}
				onblur={() => {
					if (pending !== '' && listed.length === 0) {
						put(value + pending);
						pending = '';
					}
				}}
			/>
		</div>
		{#if listed.length > 0}
			<div class="list" role="listbox" id={`${id}-list`} aria-label="Fields">
				{#each listed as field, i (field)}
					<button
						type="button"
						role="option"
						aria-selected={i === chosen}
						class="option"
						onmousedown={(event) => {
							event.preventDefault();
						}}
						onclick={() => {
							pick(field);
						}}
						><span class="option-name">{field}</span><span class="option-note"
							>{fieldNotes[field]}</span
						></button
					>
				{/each}
			</div>
		{/if}
	{/if}
	<button
		type="button"
		class="as-text"
		aria-pressed={asText}
		onclick={() => {
			asText = !asText;
		}}>{asText ? 'CHIPS' : 'TEXT'}</button
	>
</div>
<p class="visually-hidden" id={`${id}-hint`}>
	Type to add to the end; type &lt; for a field; Backspace takes the last piece off.
</p>

<style>
	.field {
		position: relative;
		display: flex;
		align-items: center;
		gap: 6px;
		min-height: calc(48px * var(--step));
		box-sizing: border-box;
		padding: calc(8px * var(--step)) 8px calc(8px * var(--step)) calc(12px * var(--step));
		border-radius: var(--radius-16);
		background: var(--card);
		border: 1px solid var(--edge-field);
		font-family: var(--font-mono);
	}

	.field.main {
		min-height: calc(60px * var(--step));
		border: 1.5px solid var(--fox-line);
		box-shadow: 3px 3px 0 color-mix(in srgb, var(--misprint) 50%, transparent);
	}

	.field.faulty {
		border-color: var(--problem-edge);
	}

	.field:focus-within {
		outline: 2px solid var(--fox-line);
		outline-offset: 2px;
	}

	.chips {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 4px;
		cursor: text;
	}

	.chip {
		display: inline-flex;
		align-items: center;
		gap: 2px;
		height: calc(30px * var(--step));
		padding: 0 4px 0 calc(10px * var(--step));
		border-radius: 8px;
		background: color-mix(in srgb, var(--fox) 14%, transparent);
		color: var(--fox-text);
		font-size: calc(13px * var(--step));
		font-weight: 700;
		white-space: pre;
	}

	.drop {
		display: grid;
		place-items: center;
		width: 20px;
		height: 20px;
		border: 0;
		border-radius: 50%;
		background: none;
		color: inherit;
		opacity: 0.55;
		cursor: pointer;
	}

	.chip:hover .drop,
	.drop:focus-visible {
		opacity: 1;
	}

	.drop:hover {
		background: color-mix(in srgb, var(--fox) 22%, transparent);
	}

	.slash {
		padding: 0 4px;
		color: var(--text-meta);
		font-size: calc(18px * var(--step));
		font-weight: 300;
	}

	.literal {
		font-family: var(--font-mono);
		color: var(--text);
		font-size: calc(14px * var(--step));
		font-weight: 500;
		white-space: pre;
	}

	.bracket {
		padding: 0 1px;
		color: var(--fox-text);
		font-size: calc(17px * var(--step));
		font-weight: 700;
	}

	.fault {
		text-decoration: underline wavy var(--problem-edge);
		text-underline-offset: 4px;
		outline: 1.5px solid var(--problem-edge);
		outline-offset: 1px;
		border-radius: 6px;
	}

	.typing {
		flex: 1;
		min-width: 3ch;
		height: calc(30px * var(--step));
		padding: 0;
		border: 0;
		background: transparent;
		color: var(--fox-text);
		caret-color: var(--fox);
		font: 600 calc(14px * var(--step)) var(--font-mono);
		outline: none;
	}

	.text {
		flex: 1;
		min-width: 0;
		height: calc(32px * var(--step));
		border: 0;
		background: transparent;
		color: var(--text);
		caret-color: var(--fox);
		font: 500 calc(14px * var(--step)) var(--font-mono);
		outline: none;
	}

	.as-text {
		align-self: flex-start;
		flex: none;
		height: 24px;
		padding: 0 8px;
		border: 1px solid var(--edge-field);
		border-radius: var(--radius-pill);
		background: none;
		color: var(--text-meta);
		font: 600 10px var(--font-mono);
		letter-spacing: 0.08em;
		cursor: pointer;
	}

	.as-text:hover {
		color: var(--text);
	}

	.list {
		position: absolute;
		left: 0;
		top: calc(100% + 6px);
		z-index: 5;
		width: min(340px, 100%);
		max-height: 320px;
		overflow: auto;
		padding: 6px;
		border-radius: var(--radius-16);
		background: var(--card-raised);
		border: 1px solid var(--edge-raised);
		box-shadow: var(--shadow-float);
	}

	.option {
		width: 100%;
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 12px;
		padding: 8px 10px;
		border: 0;
		border-radius: 10px;
		background: none;
		color: var(--text);
		text-align: left;
		cursor: pointer;
	}

	.option[aria-selected='true'],
	.option:hover {
		background: color-mix(in srgb, var(--fox) 14%, transparent);
	}

	.option-name {
		font: 700 13px var(--font-mono);
		color: var(--fox-text);
	}

	.option-note {
		font: 400 11px var(--font-sans);
		color: var(--text-meta);
	}

	/* A phone: the board's smaller chips, which wrap. */
	@media (max-width: 720px) {
		.chip {
			height: 26px;
			font-size: 12px;
		}

		.literal,
		.typing {
			font-size: 12px;
		}
	}
</style>
