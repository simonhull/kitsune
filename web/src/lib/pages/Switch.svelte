<script lang="ts">
	interface Props {
		label: string;
		on: boolean;
		/** Under the label: what it does, and how things stand. */
		note: string;
		disabled?: boolean;
		toggle: (on: boolean) => void;
	}

	let { label, on, note, disabled = false, toggle }: Props = $props();

	const id = $props.id();
</script>

<div class="switch" class:disabled>
	<input
		type="checkbox"
		role="switch"
		id={`${id}-switch`}
		checked={on}
		{disabled}
		aria-describedby={`${id}-note`}
		onchange={(event) => {
			toggle(event.currentTarget.checked);
		}}
	/>
	<span class="track" aria-hidden="true"><span class="knob"></span></span>
	<span class="words">
		<label class="label" for={`${id}-switch`}>{label}</label>
		<span class="note" id={`${id}-note`}>{note}</span>
	</span>
</div>

<style>
	.switch {
		position: relative;
		display: flex;
		align-items: flex-start;
		gap: 14px;
		cursor: pointer;
	}

	.switch.disabled {
		cursor: default;
	}

	input {
		position: absolute;
		z-index: 1;
		width: 40px;
		height: 24px;
		margin: 2px 0 0;
		opacity: 0;
		cursor: pointer;
	}

	input:disabled {
		cursor: default;
	}

	.track {
		flex: none;
		position: relative;
		width: 40px;
		height: 24px;
		margin-top: 2px;
		border-radius: var(--radius-pill);
		background: var(--edge-strong);
		transition: background-color var(--morph-fast) var(--curve-morph);
	}

	.knob {
		position: absolute;
		left: 3px;
		top: 3px;
		width: 18px;
		height: 18px;
		border-radius: 50%;
		background: var(--text);
		transition: translate var(--morph-fast) var(--curve-morph);
	}

	input:checked + .track {
		background: var(--fox);
	}

	input:checked + .track .knob {
		translate: 16px 0;
		background: var(--on-fox-ink);
	}

	input:focus-visible + .track {
		outline: 2px solid var(--fox-line);
		outline-offset: 2px;
	}

	input:disabled + .track {
		opacity: 0.4;
	}

	.label {
		display: block;
		cursor: pointer;
		font-size: var(--type-body);
		font-weight: 600;
	}

	.note {
		display: block;
		margin-top: 4px;
		font-size: 13px;
		line-height: 1.5;
		color: var(--text-meta);
	}

	@media (prefers-reduced-motion: reduce) {
		.track,
		.knob {
			transition: none;
		}
	}
</style>
