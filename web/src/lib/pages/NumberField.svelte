<script lang="ts">
	interface Props {
		id: string;
		label: string;
		/** Whose number this is, for anyone not reading along a row. */
		whose?: string | undefined;
		value: string;
		/** What it started as. It can change, but it can't be cleared: emptied, it goes back to this. */
		was: string;
		/** The total beside it. Read-only: no edit reaches it. */
		total?: number | undefined;
		/** The label is a column heading on a wide screen. */
		columned?: boolean;
		invalid?: boolean;
	}

	let {
		id,
		label,
		whose,
		value = $bindable(),
		was,
		total,
		columned = false,
		invalid = false
	}: Props = $props();

	function settle(): void {
		if (value.trim() === '') {
			value = was;
		}
	}
</script>

<div class="number" class:columned>
	<label for={id}>
		<span class="name">{label}</span>
		{#if whose !== undefined}<span class="visually-hidden">of {whose}</span>{/if}
	</label>
	<div class="box">
		<input
			{id}
			type="text"
			inputmode="numeric"
			autocomplete="off"
			placeholder={was}
			aria-invalid={invalid}
			aria-describedby={invalid ? `${id}-problem` : undefined}
			bind:value
			onblur={settle}
		/>
		{#if total !== undefined}<span class="total">of {total}</span>{/if}
	</div>
</div>

<style>
	.number {
		display: flex;
		flex-direction: column;
		gap: 6px;
		min-width: 0;
	}

	label {
		font-size: 13px;
		font-weight: 600;
		color: var(--text-meta);
	}

	.box {
		display: flex;
		align-items: center;
		gap: 8px;
	}

	input {
		width: 76px;
		height: var(--target);
		padding: 0 14px;
		border: 1px solid var(--edge-strong);
		border-radius: var(--radius-pill);
		background: rgb(10 9 8 / 30%);
		color: var(--text);
		font-family: var(--font-mono);
		font-size: 16px;
		font-variant-numeric: tabular-nums;
	}

	input[aria-invalid='true'] {
		border-color: var(--fox);
	}

	.total {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		color: var(--text-label);
		white-space: nowrap;
	}

	@media (min-width: 821px) {
		.columned .name {
			position: absolute;
			width: 1px;
			height: 1px;
			overflow: hidden;
			clip-path: inset(50%);
		}
	}
</style>
