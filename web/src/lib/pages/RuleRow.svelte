<!--
	One rule of the rules editor: its joiner in a gutter (WHERE, AND, OR), the field and how it
	compares as pill selects, the value its field takes, and × to take it out. A text rule with
	nothing typed is marked NOT COUNTED YET, and a playlist rule whose playlist is gone PLAYLIST GONE.
	On a phone it's a card: the joiner, field and how on one line, the value under them.
-->
<script lang="ts">
	import { counts, fields, formats, ops, specOf } from '#lib/library/smartlist.ts';
	import type { SmartField, SmartOp, SmartRule } from '#lib/library/source.ts';
	import PillSelect from './PillSelect.svelte';
	import RuleStars from './RuleStars.svelte';

	interface Props {
		rule: SmartRule;
		/** WHERE, AND, OR, or nothing for a group's first. */
		join: string;
		/** The joiner in the fox: every one but WHERE. */
		joinFox: boolean;
		/** Inside a group: a narrower gutter and smaller pills. */
		inGroup?: boolean;
		phone?: boolean;
		/** The person's playlists a Playlist rule may name, other than this one. */
		playlists: readonly { id: string; name: string }[];
		onfield: (field: SmartField, picks: readonly string[]) => void;
		onop: (op: SmartOp) => void;
		onvalue: (value: string | number) => void;
		onremove: () => void;
	}

	let {
		rule,
		join,
		joinFox,
		inGroup = false,
		phone = false,
		playlists,
		onfield,
		onop,
		onvalue,
		onremove
	}: Props = $props();

	const spec = $derived(specOf(rule.field));
	const fieldOptions = fields.map(({ field, label }) => [field, label] as const);
	const opOptions = $derived(ops[spec.kind]);
	const gone = $derived(
		rule.field === 'playlist' && !playlists.some(({ id }) => id === String(rule.value))
	);
	const pickOptions = $derived(
		rule.field === 'format'
			? formats.map((f) => [f, f] as const)
			: [
					...playlists.map(({ id, name }) => [id, name] as const),
					...(gone ? [[String(rule.value), 'A playlist that’s gone'] as const] : [])
				]
	);
	const picksFor = (field: SmartField): readonly string[] =>
		field === 'format' ? formats : playlists.map(({ id }) => id);
	const done = $derived(counts(rule));
	const unit = $derived(
		spec.kind === 'days' ? 'days' : spec.kind === 'min' ? 'minutes' : (spec.unit ?? '')
	);

	function number(event: Event & { currentTarget: HTMLInputElement }): void {
		const n = Math.max(0, Math.trunc(Number(event.currentTarget.value)));
		if (Number.isFinite(n)) {
			onvalue(n);
		}
	}
</script>

<div class="rule" class:in-group={inGroup} class:phone>
	<span class="join" class:fox={joinFox}>{join}</span>
	<span class="pickers">
		<PillSelect
			value={rule.field}
			options={fieldOptions}
			label="Field"
			width={phone ? undefined : inGroup ? 140 : 150}
			size={inGroup ? 'small' : 'large'}
			strong
			onchange={(field) => {
				onfield(field, picksFor(field));
			}}
		/>
		<PillSelect
			value={rule.op}
			options={opOptions}
			label="How"
			width={phone ? undefined : inGroup ? 150 : 160}
			size={inGroup ? 'small' : 'large'}
			onchange={onop}
		/>
	</span>
	<span class="value">
		{#if spec.kind === 'text'}
			<input
				class="text"
				class:counted={done}
				value={String(rule.value)}
				aria-label={spec.label}
				placeholder={spec.hint}
				maxlength="200"
				autocomplete="off"
				oninput={(event) => {
					onvalue(event.currentTarget.value);
				}}
			/>
		{:else if spec.kind === 'stars'}
			<RuleStars halves={Number(rule.value)} {phone} onpick={onvalue} />
		{:else if spec.kind === 'pick'}
			<PillSelect
				value={String(rule.value)}
				options={pickOptions}
				label={spec.label}
				look="value"
				size={inGroup ? 'small' : 'large'}
				onchange={onvalue}
			/>
		{:else}
			<input
				class="number"
				class:short={spec.kind === 'days' || spec.kind === 'min'}
				type="number"
				inputmode="numeric"
				min="0"
				value={Number(rule.value)}
				aria-label={spec.kind === 'days' ? 'Days' : spec.kind === 'min' ? 'Minutes' : spec.label}
				oninput={number}
			/>
			{#if unit !== ''}
				<span class:mono={spec.kind === 'num'} class="unit">{unit}</span>
			{/if}
		{/if}
		{#if !done}
			<span class="flag">NOT COUNTED YET</span>
		{:else if gone}
			<span class="flag">PLAYLIST GONE</span>
		{/if}
	</span>
	<button type="button" class="remove" aria-label="Take this rule out" onclick={onremove}
		><svg
			width="14"
			height="14"
			viewBox="0 0 24 24"
			fill="none"
			stroke="currentColor"
			stroke-width="2.4"
			stroke-linecap="round"
			aria-hidden="true"><path d="M6 6l12 12M18 6L6 18"></path></svg
		></button
	>
</div>

<style>
	.rule {
		display: flex;
		align-items: center;
		gap: 8px;
		min-height: 52px;
	}

	.in-group {
		min-height: 50px;
	}

	.join {
		flex: none;
		width: calc(58px * var(--step));
		font-family: var(--font-mono);
		font-size: 11px;
		font-weight: 700;
		letter-spacing: 0.12em;
		color: var(--text-meta);
		transform: rotate(-2deg);
	}

	.in-group .join {
		width: calc(34px * var(--step));
		transform: none;
	}

	.join.fox {
		color: var(--fox-text);
	}

	.pickers {
		display: contents;
	}

	.value {
		flex: 1;
		min-width: 0;
		display: flex;
		align-items: center;
		gap: 8px;
	}

	input {
		height: 40px;
		box-sizing: border-box;
		border: 0;
		border-bottom: 2px solid var(--edge-strong);
		border-radius: 10px 10px 0 0;
		background: var(--hover);
		outline: none;
		color: var(--text);
		font: 600 15px var(--font-sans);
	}

	.in-group input {
		height: 38px;
	}

	input:focus-visible {
		background: color-mix(in srgb, var(--fox) 8%, var(--hover));
	}

	.text {
		flex: 1;
		min-width: 0;
		padding: 0 14px;
	}

	.text::placeholder {
		font-style: italic;
		color: var(--text-meta);
	}

	.text.counted,
	.number {
		border-bottom-color: var(--fox-line);
	}

	.number {
		width: calc(96px * var(--step));
		padding: 0 12px;
		font-family: var(--font-mono);
	}

	.number.short {
		width: calc(80px * var(--step));
	}

	.unit {
		font-size: 14px;
		color: var(--text-secondary);
	}

	.unit.mono {
		font-family: var(--font-mono);
		font-size: 12px;
		color: var(--text-meta);
	}

	.flag {
		flex: none;
		font-family: var(--font-mono);
		font-size: 10px;
		letter-spacing: 0.1em;
		color: var(--text-meta);
	}

	.remove {
		flex: none;
		display: grid;
		place-items: center;
		width: 40px;
		height: 40px;
		border: 0;
		border-radius: 50%;
		background: none;
		color: var(--text-meta);
	}

	.remove:hover {
		background: var(--hover);
		color: var(--text);
	}

	/* The phone board: a card, the joiner, field and how on one line and the value under them. */
	.phone {
		display: grid;
		grid-template-columns: 40px minmax(0, 1fr) 44px;
		align-items: center;
		gap: 4px 6px;
		padding: 10px 6px 10px 12px;
		border-radius: var(--radius-16);
		background: var(--card);
	}

	.phone .join {
		width: auto;
		font-size: 10px;
		letter-spacing: 0.1em;
		transform: none;
	}

	.phone .pickers {
		grid-column: 2;
		grid-row: 1;
		display: flex;
		gap: 6px;
		min-width: 0;
	}

	/* Field and how share the line, giving up their room before they wrap. */
	.phone .pickers :global(.pill) {
		flex: 0 1 auto;
		min-width: 0;
	}

	.phone .pickers :global(select) {
		width: 100%;
		min-width: 0;
		height: 36px;
		padding: 0 24px 0 12px;
	}

	.phone .pickers :global(svg) {
		right: 9px;
	}

	.phone .remove {
		grid-column: 3;
		grid-row: 1;
		width: 44px;
		height: 44px;
	}

	.phone .value {
		grid-column: 2 / 4;
		grid-row: 2;
		padding-right: 6px;
	}

	.phone.in-group {
		padding: 0;
		background: none;
	}
</style>
