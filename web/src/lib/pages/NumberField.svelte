<script lang="ts">
	interface Props {
		id: string;
		/** What a phone shows beside it, where the row can't say it by position. */
		label: string;
		/** Whose number this is, for anyone not reading along a row. */
		whose: string;
		value: string;
		changed: boolean;
		invalid: boolean;
		/** The disc is quieter than the track, and narrower. */
		quiet?: boolean;
	}

	let { id, label, whose, value = $bindable(), changed, invalid, quiet = false }: Props = $props();
</script>

<label class="number" class:changed class:invalid class:quiet>
	<span class="name">{label}</span><span class="visually-hidden"> of {whose}</span>
	<input
		{id}
		type="text"
		inputmode="numeric"
		autocomplete="off"
		aria-invalid={invalid}
		aria-describedby={invalid ? `${id}-problem` : undefined}
		bind:value
	/>
</label>

<style>
	.number {
		display: flex;
		align-items: center;
		gap: 10px;
		min-width: 0;
	}

	/* A wide row says which is which by position. It's still read out. */
	.name {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip-path: inset(50%);
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		color: var(--text-meta);
	}

	input {
		width: 40px;
		height: 40px;
		padding: 0;
		border: 0;
		border-bottom: 1px solid var(--edge-field);
		outline: none;
		background: transparent;
		color: var(--text);
		font-family: var(--font-mono);
		font-size: 13px;
		font-variant-numeric: tabular-nums;
		text-align: center;
	}

	.quiet input {
		width: 28px;
		color: var(--text-meta);
	}

	input:focus {
		border-bottom-color: var(--text);
	}

	.changed input {
		border-bottom-color: var(--fox-line);
	}

	.invalid input {
		border-bottom-color: var(--problem-edge);
	}

	@media (max-width: 820px) {
		.name {
			position: static;
			width: auto;
			height: auto;
			clip-path: none;
		}

		input,
		.quiet input {
			width: 44px;
			height: 44px;
			border-bottom: 0;
			color: var(--text);
			font-size: 14px;
		}

		.quiet input {
			color: var(--text-secondary);
		}

		.changed input {
			color: var(--fox-text);
		}

		.invalid input {
			color: var(--problem);
		}
	}
</style>
