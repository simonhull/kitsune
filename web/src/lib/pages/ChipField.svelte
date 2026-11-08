<script lang="ts">
	import Icon from '#lib/shell/Icon.svelte';

	interface Props {
		id: string;
		/** What the field is, read out. */
		label: string;
		values: string[];
		placeholder?: string;
		/** A typed value ends at a comma. Off for names, which may hold one. */
		commas?: boolean;
	}

	let { id, label, values = $bindable(), placeholder = 'Add', commas = false }: Props = $props();

	let typing = $state('');
	let input: HTMLInputElement | undefined = $state();

	/** Takes what's been typed, once. A value from a file stays whole, whatever it holds. */
	function take(): void {
		const typed = commas ? typing.split(',') : [typing];
		for (const part of typed) {
			const value = part.trim().replace(/\s+/g, ' ');
			const folded = value.toLowerCase();
			if (value !== '' && !values.some((other) => other.toLowerCase() === folded)) {
				values = [...values, value];
			}
		}
		typing = '';
	}

	function key(event: KeyboardEvent): void {
		if ((commas && event.key === ',') || (event.key === 'Enter' && typing.trim() !== '')) {
			event.preventDefault();
			take();
		} else if (event.key === 'Backspace' && typing === '' && values.length > 0) {
			values = values.slice(0, -1);
		}
	}

	function remove(gone: number): void {
		values = values.filter((_, index) => index !== gone);
		input?.focus();
	}
</script>

<div class="chips">
	<!-- By place: a file can hold the same value twice. -->
	{#each values as value, index (index)}
		<button
			type="button"
			class="chip"
			aria-label={`Remove ${value}`}
			onclick={() => {
				remove(index);
			}}
		>
			<span>{value}</span>
			<Icon name="close" size={12} />
		</button>
	{/each}
	<input
		{id}
		type="text"
		autocomplete="off"
		aria-label={label}
		{placeholder}
		bind:this={input}
		bind:value={typing}
		onkeydown={key}
		onblur={take}
	/>
</div>

<style>
	.chips {
		display: flex;
		align-items: center;
		gap: 6px;
		min-width: 0;
		min-height: 32px;
		overflow-x: auto;
		scrollbar-width: none;
	}

	.chip {
		flex: none;
		display: flex;
		align-items: center;
		gap: 6px;
		height: 32px;
		padding: 0 10px 0 12px;
		border: 0;
		border-radius: var(--radius-pill);
		background: var(--pressed);
		font-size: 14px;
		font-weight: 600;
		white-space: nowrap;
	}

	/* A chip is 44px to press however it looks, and it's in the way of nothing around it. */
	.chip {
		position: relative;
	}

	.chip::after {
		content: '';
		position: absolute;
		inset: -6px 0;
	}

	input {
		flex: 1;
		min-width: 64px;
		height: 32px;
		padding: 0;
		border: 0;
		outline: none;
		background: transparent;
		color: inherit;
		font: inherit;
		font-size: 14px;
	}

	input::placeholder {
		color: var(--text-label);
	}
</style>
