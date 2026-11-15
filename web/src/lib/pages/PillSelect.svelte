<!--
	A native select drawn as the rules boards' pills: outlined on the ground (a field, how it
	compares, a limit's unit), in the fox wash (all or any), or underlined as a value. Native, so a
	phone opens its own picker and a keyboard works as it always does.
-->
<script lang="ts" generics="T extends string">
	interface Props {
		value: T;
		options: readonly (readonly [T, string])[];
		label: string;
		look?: 'outline' | 'fox' | 'value';
		/** Its width in CSS pixels at the base step; undefined fits its words. */
		width?: number | undefined;
		size?: 'large' | 'small';
		strong?: boolean;
		onchange: (value: T) => void;
	}

	let {
		value,
		options,
		label,
		look = 'outline',
		width,
		size = 'large',
		strong = false,
		onchange
	}: Props = $props();
</script>

<span class={['pill', look, size]} class:strong>
	<select
		aria-label={label}
		{value}
		style:width={width === undefined ? undefined : `calc(${String(width)}px * var(--step))`}
		onchange={(event) => {
			onchange(event.currentTarget.value as T);
		}}
	>
		{#each options as [v, words] (v)}
			<option value={v}>{words}</option>
		{/each}
	</select>
	<svg
		width="11"
		height="11"
		viewBox="0 0 24 24"
		fill="none"
		stroke="currentColor"
		stroke-width="3"
		stroke-linecap="round"
		aria-hidden="true"><path d="M6 9l6 6 6-6"></path></svg
	>
</span>

<style>
	.pill {
		position: relative;
		flex: none;
		display: inline-flex;
		align-items: center;
	}

	select {
		appearance: none;
		height: 40px;
		max-width: 100%;
		padding: 0 30px 0 14px;
		border: 1px solid var(--edge-strong);
		border-radius: var(--radius-pill);
		background: var(--ground);
		color: var(--text);
		font: 500 14px var(--font-sans);
		cursor: pointer;
		text-overflow: ellipsis;
	}

	.small select {
		height: 38px;
		padding: 0 30px 0 12px;
	}

	.strong select {
		font-weight: 700;
	}

	select:hover {
		border-color: var(--edge-hover);
	}

	select:focus-visible {
		outline: none;
		box-shadow: 0 0 0 3px var(--fox-line);
	}

	.fox select {
		border: 0;
		background: color-mix(in srgb, var(--fox) 14%, transparent);
		color: var(--fox-text);
		font-size: 17px;
		font-weight: 800;
		padding: 0 34px 0 14px;
	}

	.fox.small select {
		height: 34px;
		font-size: 14px;
		padding: 0 30px 0 12px;
	}

	.value select {
		min-width: 150px;
		border: 0;
		border-bottom: 2px solid var(--fox-line);
		border-radius: 10px 10px 0 0;
		background: var(--hover);
		font-size: 15px;
		font-weight: 600;
	}

	svg {
		position: absolute;
		right: 12px;
		pointer-events: none;
		color: var(--text-meta);
	}

	.fox svg {
		color: var(--fox-text);
		right: 13px;
	}
</style>
