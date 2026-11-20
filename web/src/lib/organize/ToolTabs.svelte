<!--
	The head of Server's library tools: the way back to Server, and Organize · Duplicates, the two
	tabs that head both pages (plans 068 and 069). The tabs are one strip across the pages that hold
	them: going between them it never leaves, and only its fill flows to the tab pressed (plan 071).
-->
<script lang="ts">
	import { resolve } from '$app/paths';
	import { morph } from '#lib/morph/registry.ts';

	interface Props {
		current: 'organize' | 'duplicates';
		/** At a phone's width the tabs are smaller, and sit at the far end. */
		phone?: boolean;
	}

	let { current, phone = false }: Props = $props();
</script>

<div class="crumbs" class:phone>
	<a class="crumb mono" href={resolve('/server')}>YOU › SERVER</a>
	<nav
		class="tabs"
		aria-label="Library tools"
		data-morph-part="intro"
		data-morph-key="tools"
		use:morph={{ id: 'tabs:tools', slot: 'hero', kind: 'strip' }}
	>
		<span class="fill" data-strip-fill aria-hidden="true"></span>
		<a
			class="tab"
			class:on={current === 'organize'}
			href={resolve('/server/organize')}
			aria-current={current === 'organize' ? 'page' : undefined}>Organize</a
		>
		<a
			class="tab"
			class:on={current === 'duplicates'}
			href={resolve('/server/duplicates')}
			aria-current={current === 'duplicates' ? 'page' : undefined}>Duplicates</a
		>
	</nav>
</div>

<style>
	.mono {
		font-family: var(--font-mono);
	}

	.crumbs {
		display: flex;
		align-items: center;
		gap: 16px;
		flex-wrap: wrap;
	}

	.crumb {
		font-size: calc(12px * var(--step));
		letter-spacing: 0.08em;
		color: var(--text-meta);
		text-decoration: none;
	}

	.crumb:hover {
		color: var(--fox-text);
	}

	.tabs {
		position: relative;
		display: flex;
		gap: 4px;
		padding: 4px;
		border-radius: var(--radius-pill);
		border: 1px solid var(--edge);
	}

	.tab {
		position: relative;
		height: calc(32px * var(--step));
		line-height: calc(32px * var(--step));
		padding: 0 14px;
		border-radius: var(--radius-pill);
		font-size: calc(14px * var(--step));
		font-weight: 600;
		color: var(--text-secondary);
		text-decoration: none;
	}

	.tab:hover {
		color: var(--text);
	}

	.tab.on {
		background: var(--fill);
		color: var(--text);
		font-weight: 700;
	}

	/* The fill under the tab that's on, while it flows there from the last one: then the tab's own. */
	.fill {
		position: absolute;
		border-radius: var(--radius-pill);
		background: var(--fill);
		opacity: 0;
		pointer-events: none;
	}

	.tabs:global([data-strip-flowing]) .fill {
		opacity: 1;
	}

	.tabs:global([data-strip-flowing]) .tab.on {
		background: transparent;
	}

	.phone {
		justify-content: space-between;
	}

	.phone .tab {
		height: 28px;
		line-height: 28px;
		padding: 0 12px;
		font-size: 13px;
	}
</style>
