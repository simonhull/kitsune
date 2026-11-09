<script lang="ts">
	import {
		allClear,
		filesState,
		issueGroups,
		splitPath,
		warningLine
	} from '#lib/library/health.ts';
	import type { Load } from '#lib/library/library.svelte.ts';
	import type { IssueKind, LibraryFacts, ScanIssues } from '#lib/library/source.ts';
	import Icon from '#lib/shell/Icon.svelte';

	/** The board's list: enough to recognize what's there, with the rest a tap away. */
	const shownAtFirst = 6;

	interface Props {
		facts: LibraryFacts;
		admin: boolean;
		issues: Load<ScanIssues>;
		/** The list was asked for again and is still behind the counts. */
		gaveUp: boolean;
		/** Asks for the list again. */
		retry: () => void;
	}

	let { facts, admin, issues, gaveUp, retry }: Props = $props();

	const list = $derived(issues.kind === 'ready' ? issues.value : null);
	const groups = $derived(issueGroups(facts.issueCounts, list?.issues ?? []));
	const warnings = $derived(facts.issueCounts.warnings);
	const clear = $derived(groups.length === 0 ? allClear(facts) : null);

	let opened = $state<ReadonlySet<IssueKind>>(new Set());
	let warningsOpen = $state(false);

	function open(kind: IssueKind): void {
		opened = new Set([...opened, kind]);
	}
</script>

<section class="panel" aria-label="Issues">
	{#if clear !== null}
		<span class="fox-mark"><Icon name="fox" size={420} /></span>
		<div class="calm">
			<h2>{clear.title}</h2>
			<p class="note">{clear.note}</p>
		</div>
	{/if}

	{#each groups as group (group.kind)}
		{@const shown = opened.has(group.kind) ? group.files : group.files.slice(0, shownAtFirst)}
		{@const files = filesState(admin, issues.kind, group.files.length, gaveUp)}
		<section class="group" aria-label={group.title}>
			<div class="heading">
				<h2>{group.title}</h2>
				<span class="tally">{group.tally}</span>
			</div>
			{#if files === 'loading'}
				<p class="files-status" role="status">Finding which files…</p>
			{:else if files === 'failed'}
				<p class="files-status unavailable" role="status">
					Couldn't load the list of files.
					<button type="button" class="more" onclick={retry}>Try again</button>
				</p>
			{/if}
			<p class="note">{group.note}</p>
			{#if shown.length > 0}
				<ul>
					<!-- Two roots can hold the same relative path, so a path is no key. -->
					{#each shown as file, index (index)}
						{#if file.kind === 'unsupported'}
							{@const { folder, name } = splitPath(file.path)}
							<li class="stacked" title={file.path}>
								{#if folder !== ''}
									<span class="folder">{folder}</span>
								{/if}
								<span class="path">{name}</span>
							</li>
						{:else}
							<li>
								<span class="path" title={file.path}>{file.path}</span>
								<span class="message" title={file.message}>{file.message}</span>
							</li>
						{/if}
					{/each}
				</ul>
			{/if}
			{#if shown.length < group.files.length}
				<button type="button" class="more" onclick={() => open(group.kind)}>
					{(group.files.length - shown.length).toLocaleString('en-US')} more
				</button>
			{/if}
		</section>
	{/each}

	{#if !admin && groups.length > 0}
		<p class="note">An admin can see which files.</p>
	{/if}

	{#if admin && issues.kind === 'unavailable' && groups.length === 0}
		<p class="note unavailable" role="status">
			The list of files couldn't be loaded.
			<button type="button" class="more" onclick={retry}>Try again</button>
		</p>
	{/if}

	{#if warnings > 0}
		<div class="warned">
			<h3>{warningLine(warnings)}</h3>
			<p class="note">
				Read fine, with something odd in its tags. Nothing to do unless it looks wrong.
			</p>
			{#if list !== null && list.warnings.length > 0}
				{#if warningsOpen}
					<ul>
						{#each list.warnings as track (track.trackId)}
							<li class="stacked">
								<span class="path" title={track.path}>{track.path}</span>
								{#each track.messages as message, index (index)}
									<span class="message" title={message}>{message}</span>
								{/each}
							</li>
						{/each}
					</ul>
				{:else}
					<button type="button" class="more" onclick={() => (warningsOpen = true)}>
						{warnings === 1 ? 'Show it' : 'Show them'}
					</button>
				{/if}
			{/if}
		</div>
	{/if}
</section>

<style>
	.panel {
		position: relative;
		overflow: hidden;
		min-height: var(--panel-height);
		padding: calc(28px * var(--step)) calc(32px * var(--step));
		border-radius: var(--radius-28);
		background: var(--card);
		display: flex;
		flex-direction: column;
		gap: 26px;
	}

	/* The Health tile's mark, grown: the same fox off the same corner, with nothing left to do. */
	.fox-mark {
		display: flex;
		position: absolute;
		right: -60px;
		bottom: -80px;
		color: var(--fox-text);
		opacity: 0.18;
	}

	.calm {
		position: relative;
	}

	.group + .group {
		padding-top: 22px;
		border-top: 1px solid var(--edge-strong);
	}

	.heading {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 4px 12px;
	}

	h2 {
		font-size: var(--type-section);
		font-weight: 800;
		letter-spacing: var(--tracking-section);
		line-height: 1.3;
	}

	h3 {
		margin: 0;
		font-size: var(--type-title);
		font-weight: 700;
		letter-spacing: var(--tracking-title);
	}

	.tally {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		font-variant-numeric: tabular-nums;
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.note {
		margin-top: 8px;
		font-size: var(--type-body);
		line-height: 1.4;
		color: var(--text-secondary);
	}

	.warned .note {
		margin-top: 4px;
		font-size: calc(14px * var(--step));
	}

	/* Where the list goes, until it's there: quiet, since the heading has already said how many. */
	.files-status {
		margin-top: 8px;
		font-size: calc(14px * var(--step));
		color: var(--text-meta);
	}

	.folder {
		min-width: 0;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		font-size: calc(12px * var(--step));
		color: var(--text-meta);
	}

	.unavailable {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0 8px;
	}

	ul {
		margin: 18px 0 0;
		padding: 0;
		list-style: none;
	}

	li {
		min-height: 49px;
		padding: 6px 0;
		border-bottom: 1px solid var(--edge-card);
		display: flex;
		align-items: center;
		gap: 12px;
		font-family: var(--font-mono);
		font-size: calc(13px * var(--step));
	}

	li.stacked {
		flex-direction: column;
		align-items: stretch;
		justify-content: center;
		gap: 2px;
	}

	.path {
		flex: 1;
		min-width: 0;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.message {
		flex: 0 1 38%;
		min-width: 0;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		color: var(--text-meta);
	}

	.stacked .message {
		flex: none;
	}

	.more {
		min-height: var(--target);
		padding: 0;
		border: 0;
		background: none;
		font-size: calc(14px * var(--step));
		color: var(--text-meta);
	}

	.more:hover {
		color: var(--fox-text);
	}

	.warned {
		padding-top: 18px;
		border-top: 1px solid var(--edge-strong);
		position: relative;
	}

	@media (max-width: 720px) {
		.panel {
			padding: 24px;
		}

		/* Nothing hovers here, so a long path wraps rather than hiding its file name. */
		li {
			flex-direction: column;
			align-items: stretch;
			justify-content: center;
			gap: 2px;
		}

		.path,
		.folder,
		.message {
			flex: none;
			white-space: normal;
			overflow-wrap: anywhere;
		}

		.calm {
			min-height: 200px;
		}

		.fox-mark {
			right: -40px;
			bottom: -60px;
		}

		.fox-mark :global(svg) {
			width: 240px;
			height: 240px;
		}
	}
</style>
