<!--
	Server › Organize (plan 068): a path template, three songs drawn as it would file them, a
	preview of every move before anything happens, a run that can be stopped, and Undo for a whole
	run. Administrators only; with tag writes off it still previews, and nothing runs.
-->
<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { MediaQuery } from 'svelte/reactivity';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { library } from '#lib/library/library.svelte.ts';
	import ExampleCard from '#lib/organize/ExampleCard.svelte';
	import { organizer } from '#lib/organize/organizer.svelte.ts';
	import PreviewAlbum from '#lib/organize/PreviewAlbum.svelte';
	import TemplateField from '#lib/organize/TemplateField.svelte';
	import ToolTabs from '#lib/organize/ToolTabs.svelte';
	import { TemplateError, tryParse, type Template } from '#lib/organize/template.ts';
	import type { OrganizeFilter } from '#lib/organize/types.ts';
	import {
		countsLine,
		dockWords,
		doneWords,
		headWords,
		paceWords,
		presets,
		reasonWords,
		runRowWords,
		scopeWords,
		summaryWords,
		undoAsk,
		whenWords
	} from '#lib/organize/words.ts';
	import Switch from '#lib/pages/Switch.svelte';
	import Icon from '#lib/shell/Icon.svelte';

	const phone = new MediaQuery('(max-width: 720px)', false);
	const o = organizer;

	let presetsOpen = $state(false);
	let folderOpen = $state(false);
	let folderRoot = $state('');
	let folderTemplate = $state('');
	let folderCompilations = $state('');
	let undoing = $state('');
	let saved = $state(false);
	let now = $state(Date.now());
	let sentinel: HTMLElement | undefined = $state();

	/** The last of each template that parsed, which the example keeps while one doesn't. */
	let lastMain: Template | null = null;
	let lastComp: Template | null = null;
	const main = $derived(tryParse(o.template));
	const comp = $derived(tryParse(o.compilations));
	const goodMain = $derived.by(() => {
		if (!(main instanceof TemplateError)) {
			lastMain = main;
		}
		return lastMain;
	});
	const goodComp = $derived.by(() => {
		if (!(comp instanceof TemplateError)) {
			lastComp = comp;
		}
		return lastComp;
	});
	const fault = (
		parsed: Template | TemplateError,
		field: 'template' | 'compilations'
	): { at: number; reason: string } | null => {
		if (parsed instanceof TemplateError) {
			return { at: parsed.at, reason: parsed.reason };
		}
		const refused = o.refused;
		return refused !== null && refused.field === field
			? { at: refused.at, reason: refused.detail }
			: null;
	};
	const mainFault = $derived(fault(main, 'template'));
	const compFault = $derived(fault(comp, 'compilations'));
	const parsed = $derived(mainFault === null && compFault === null);

	const counts = $derived(o.preview?.counts ?? null);
	const filters = $derived.by((): { id: OrganizeFilter; label: string; count: number }[] => {
		if (counts === null) {
			return [];
		}
		const all = [
			{ id: 'all' as const, label: 'All moves', count: counts.moves },
			{
				id: 'rename' as const,
				label: phone.current ? 'Renamed' : 'Renamed in place',
				count: counts.renames
			},
			{ id: 'move' as const, label: 'New folder', count: counts.newFolders },
			{ id: 'conflict' as const, label: 'Conflicts', count: counts.conflicts },
			{ id: 'skipped' as const, label: 'Skipped', count: counts.skipped },
			{ id: 'inPlace' as const, label: 'In place', count: counts.inPlace }
		];
		return all.filter((f) => f.id !== 'skipped' || f.count > 0);
	});
	const run = $derived(o.run);
	const can = $derived(library.user?.canOrganize === true);
	const running = $derived(o.view === 'run' && run !== null);
	/** The head's last line: the album in hand, and what Stop does. */
	const nowLine = (album: string | undefined): string => {
		const title = albumTitle(album);
		const rest = 'STOP FINISHES THIS ALBUM, THEN STOPS · MUSIC KEEPS PLAYING';
		return title === '' ? rest : `NOW · ${title.toUpperCase()} · ${rest}`;
	};
	const albumTitle = (id: string | undefined): string =>
		id === undefined ? '' : (o.albums.find((a) => a.id === id)?.title ?? '');
	const standing = (id: string): 'moved' | 'moving' | 'waiting' | undefined => {
		if (!running) {
			return undefined;
		}
		const at = o.runOrder.indexOf(id);
		if (at < 0) {
			return undefined;
		}
		return at < o.albumsDone ? 'moved' : at === o.albumsDone ? 'moving' : 'waiting';
	};
	const shownAlbums = $derived(
		running ? o.albums.filter((a) => o.runOrder.includes(a.id)) : o.albums
	);
	const percent = $derived(
		run === null || run.total === 0 ? 0 : Math.round((run.done / run.total) * 1000) / 10
	);
	const elapsed = $derived(
		run === null ? 0 : now - Math.min(Date.parse(run.startedAt), o.heardSince)
	);
	const dock = $derived(counts === null ? null : dockWords(o.moving, counts, o.picked.size > 0));

	onMount(() => {
		void o.open().then(() => {
			const id = page.url.searchParams.get('preview');
			if (id !== null && o.view !== 'run') {
				void o.openPreview(id);
			}
		});
		const clock = setInterval(() => {
			now = Date.now();
		}, 1000);
		return () => {
			clearInterval(clock);
		};
	});

	onDestroy(() => {
		presetsOpen = false;
	});

	$effect(() => {
		// The list reads its next page as its end comes near.
		const end = sentinel;
		if (end === undefined) {
			return;
		}
		const watch = new IntersectionObserver(
			(entries) => {
				if (entries.some((e) => e.isIntersecting)) {
					void o.more();
				}
			},
			{ rootMargin: '800px' }
		);
		watch.observe(end);
		return () => {
			watch.disconnect();
		};
	});

	async function preview(): Promise<void> {
		presetsOpen = false;
		await o.startPreview();
	}

	async function save(): Promise<void> {
		saved = await o.saveTemplates();
		setTimeout(() => {
			saved = false;
		}, 2400);
	}

	function wholeLibrary(): void {
		void o.startPreview();
	}

	function openFolder(): void {
		folderOpen = !folderOpen;
		const root = folderRoot === '' ? (o.roots[0] ?? '') : folderRoot;
		folderRoot = root;
		const own = o.rootTemplates[root];
		folderTemplate = own?.template ?? o.template;
		folderCompilations = own?.compilations ?? o.compilations;
	}
</script>

<svelte:head>
	<title>Organize · Kitsune</title>
</svelte:head>

{#if !library.admin}
	<p class="gone">Organize is for administrators. <a href={resolve('/you')}>Back to You</a></p>
{:else}
	<div class="page-frame organize" class:phone={phone.current}>
		<header class="intro">
			<div class="intro-words">
				<ToolTabs current="organize" phone={phone.current} />
				<h1 data-morph-part="intro" data-morph-key="headline" data-morph-headline>
					Organize<span class="dot">.</span>
				</h1>
				<p class="counts mono" data-testid="counts">
					{counts === null ? ' ' : countsLine(counts, phone.current)}
				</p>
			</div>
			{#if !phone.current}
				<p class="promise mono">
					MOVES FILES ON DISK · ONLY ADMINS · NEVER OVERWRITES · EVERY RUN CAN BE UNDONE
				</p>
			{/if}
		</header>

		<div class="body">
			<section class="left" aria-label="Template">
				{#if running}
					<div class="summary">
						<span class="label mono">TEMPLATE</span>
						<span class="summary-text mono">{o.template}</span>
						<span class="label mono">COMPILATIONS</span>
						<span class="summary-text mono">{o.compilations}</span>
					</div>
				{:else if phone.current && o.view !== 'template'}
					<!-- A phone's preview: the template is one field, and the list follows it. -->
					<TemplateField
						value={o.template}
						label="Template"
						main
						fault={mainFault}
						onchange={(value) => {
							o.template = value;
							o.refused = null;
						}}
					/>
					<div class="again">
						<button
							type="button"
							class="text-button"
							disabled={!parsed || o.loading}
							onclick={() => void preview()}>Preview again</button
						>
						<button
							type="button"
							class="text-button quiet"
							onclick={() => {
								o.leave();
							}}>All the settings</button
						>
					</div>
				{:else}
					<div class="label-row">
						<span class="label mono">TEMPLATE · EVERY FOLDER</span>
						<button
							type="button"
							class="presets"
							aria-expanded={presetsOpen}
							aria-haspopup="menu"
							onclick={() => {
								presetsOpen = !presetsOpen;
							}}>Presets <Icon name="chevron-down" size={12} /></button
						>
						{#if presetsOpen}
							<div class="preset-menu" role="menu" aria-label="Presets">
								{#each presets as preset (preset.name)}
									<button
										type="button"
										role="menuitem"
										class="preset"
										class:current={preset.template === o.template}
										onclick={() => {
											o.template = preset.template;
											presetsOpen = false;
										}}
										><span class="preset-name">{preset.name}</span><span class="preset-line mono"
											>{preset.template}</span
										></button
									>
								{/each}
							</div>
						{/if}
					</div>
					<TemplateField
						value={o.template}
						label="Template"
						main
						fault={mainFault}
						onchange={(value) => {
							o.template = value;
							o.refused = null;
						}}
					/>
					{#if mainFault !== null}
						<p class="fault mono" role="alert">
							AT {mainFault.at}: {mainFault.reason.toUpperCase()}
						</p>
					{:else}
						<p class="hint mono">TYPE &lt; FOR A FIELD · [ ] GOES WHEN EMPTY · / MAKES A FOLDER</p>
					{/if}

					<span class="label mono spaced">COMPILATIONS</span>
					<TemplateField
						value={o.compilations}
						label="Compilations"
						fault={compFault}
						onchange={(value) => {
							o.compilations = value;
							o.refused = null;
						}}
					/>
					{#if compFault !== null}
						<p class="fault mono" role="alert">
							AT {compFault.at}: {compFault.reason.toUpperCase()}
						</p>
					{/if}
					{#if o.roots.length > 0}
						<button type="button" class="fold" aria-expanded={folderOpen} onclick={openFolder}
							>Use another template for one folder ›</button
						>
					{/if}
					{#if folderOpen}
						<div class="folder-card">
							<label class="folder-pick">
								<span class="label mono">FOLDER</span>
								<select
									bind:value={folderRoot}
									onchange={() => {
										const own = o.rootTemplates[folderRoot];
										folderTemplate = own?.template ?? o.template;
										folderCompilations = own?.compilations ?? o.compilations;
									}}
								>
									{#each o.roots as root (root)}
										<option value={root}>{root}</option>
									{/each}
								</select>
							</label>
							<TemplateField
								value={folderTemplate}
								label="This folder's template"
								fault={(() => {
									const p = tryParse(folderTemplate);
									return p instanceof TemplateError ? { at: p.at, reason: p.reason } : null;
								})()}
								onchange={(value) => {
									folderTemplate = value;
								}}
							/>
							<TemplateField
								value={folderCompilations}
								label="This folder's compilations"
								fault={(() => {
									const p = tryParse(folderCompilations);
									return p instanceof TemplateError ? { at: p.at, reason: p.reason } : null;
								})()}
								onchange={(value) => {
									folderCompilations = value;
								}}
							/>
							<div class="folder-buttons">
								<button
									type="button"
									class="outline small"
									disabled={tryParse(folderTemplate) instanceof TemplateError ||
										tryParse(folderCompilations) instanceof TemplateError}
									onclick={() =>
										void o.setRootTemplates(folderRoot, {
											template: folderTemplate,
											compilations: folderCompilations
										})}>Use these for {folderRoot}</button
								>
								{#if o.rootTemplates[folderRoot] !== undefined}
									<button
										type="button"
										class="text-button"
										onclick={() => void o.setRootTemplates(folderRoot, null)}
										>Use the main ones</button
									>
								{/if}
							</div>
						</div>
					{/if}

					<div class="rule"></div>
					<div class="options">
						<Switch
							label="Bring their files along"
							note="COVERS, .CUE, .LOG, .LRC AND .TXT BESIDE THE SONGS"
							on={o.options['organize.companions']}
							toggle={(on) => void o.setOption('organize.companions', on)}
						/>
						<Switch
							label="Organize after a tag edit"
							note="ONLY SONGS ALREADY WHERE THE TEMPLATE PUTS THEM"
							on={o.options['organize.on_edit']}
							toggle={(on) => void o.setOption('organize.on_edit', on)}
						/>
						<Switch
							label="Rewrite .m3u files that name a moved song"
							note="KITSUNE’S OWN PLAYLISTS FOLLOW EVERY MOVE EITHER WAY"
							on={o.options['organize.rewrite_playlists']}
							toggle={(on) => void o.setOption('organize.rewrite_playlists', on)}
						/>
					</div>

					<div class="buttons">
						<button
							type="button"
							class="fox"
							disabled={!parsed || o.loading}
							onclick={() => void preview()}>Preview</button
						>
						<button type="button" class="outline" disabled={!parsed} onclick={() => void save()}
							>{saved ? 'Saved' : 'Save template'}</button
						>
					</div>
				{/if}
				{#if o.problem !== ''}
					<p class="problem" role="alert">{o.problem}</p>
				{/if}
			</section>

			<section
				class="right"
				aria-label={o.view === 'template' ? 'Example' : o.view === 'done' ? 'The run' : 'Preview'}
			>
				{#if o.view === 'template'}
					<ExampleCard
						examples={o.examples}
						template={goodMain}
						compilations={goodComp}
						one={phone.current}
						onanother={() => void o.another()}
					/>
					{@render runsCard()}
				{:else if o.view === 'done' && run !== null}
					{@render doneCard()}
					{@render runsCard()}
				{:else}
					{#if running && run !== null}
						{@render runHead()}
					{/if}
					{#if o.preview?.scope !== undefined && !running}
						<div class="scope">
							<span>{scopeWords(o.preview.scope)}</span>
							<button type="button" class="text-button" onclick={wholeLibrary}
								>Show the whole library</button
							>
						</div>
					{/if}
					{#if !running}
						<div class="filters">
							<div class="chips" role="group" aria-label="Show">
								{#each filters as f (f.id)}
									<button
										type="button"
										class="filter"
										class:hot={f.id === 'conflict'}
										aria-pressed={o.filter === f.id}
										onclick={() => void o.refilter(f.id)}
										>{f.label}
										<span class="count mono">{f.count.toLocaleString('en-US')}</span></button
									>
								{/each}
							</div>
							<label class="search">
								<Icon name="search" size={13} />
								<input
									aria-label="Find in the preview"
									placeholder="Artist, album, title or path"
									value={o.query}
									oninput={(event) => {
										o.search(event.currentTarget.value);
									}}
								/>
							</label>
						</div>
					{/if}
					<div class="list" class:docked={!running}>
						{#each shownAlbums as album (album.id)}
							<PreviewAlbum
								{album}
								standing={standing(album.id)}
								phone={phone.current}
								picked={running || !can ? undefined : o.picked.has(album.id)}
								onpick={running || !can
									? undefined
									: () => {
											o.togglePick(album.id);
										}}
							/>
						{:else}
							{#if !o.loading}
								<p class="none">
									{o.query === '' ? 'Nothing here.' : 'Nothing in this preview holds that.'}
								</p>
							{/if}
						{/each}
						{#if o.nextCursor !== undefined}
							<div class="sentinel" bind:this={sentinel}></div>
						{/if}
					</div>
					{#if !running && dock !== null}
						<div class="dock" class:phone={phone.current}>
							{#if can}
								{#if phone.current}
									<button
										type="button"
										class="fox wide"
										disabled={o.moving === 0}
										onclick={() => void o.runIt()}
										>Run it · {dock.title.charAt(0).toLowerCase()}{dock.title.slice(1)}</button
									>
									<p class="dock-line mono">
										{counts?.conflicts ?? 0} IN CONFLICT STAY PUT · EVERY RUN CAN BE UNDONE
									</p>
								{:else}
									<span class="dock-words">
										<span class="dock-title">{dock.title}</span>
										<span class="dock-line mono">{dock.line}</span>
									</span>
									<button type="button" class="outline" onclick={() => o.leave()}>Leave it</button>
									<button
										type="button"
										class="fox"
										disabled={o.moving === 0}
										onclick={() => void o.runIt()}>Run it</button
									>
								{/if}
							{:else}
								<span class="dock-words">
									<span class="dock-title">Tag writes are off, so nothing on disk changes.</span>
									<span class="dock-line mono"
										>TURN THEM ON IN <a href={resolve('/server')}>SERVER › LIBRARY</a></span
									>
								</span>
								<button type="button" class="outline" onclick={() => o.leave()}>Leave it</button>
							{/if}
						</div>
					{/if}
				{/if}
			</section>
		</div>
	</div>
{/if}

{#snippet runHead()}
	{#if run !== null}
		<div class="head-card" role="status" aria-live="polite">
			<div class="head-row">
				<span class="head-title" data-testid="run-head">{headWords(run)}</span>
				{#if !phone.current}
					<span class="pace mono">{paceWords(run.done, run.total, elapsed)}</span>
				{/if}
				<button
					type="button"
					class="outline stop"
					disabled={run.stopping === true}
					onclick={() => void o.stop()}>{run.stopping === true ? 'Stopping…' : 'Stop'}</button
				>
			</div>
			<div
				class="bar"
				role="progressbar"
				aria-label={run.kind === 'undo' ? 'Songs put back' : 'Songs moved'}
				aria-valuemin={0}
				aria-valuemax={run.total}
				aria-valuenow={run.done}
			>
				<span style:width={`${String(percent)}%`}></span>
			</div>
			<span class="now mono">{nowLine(run.album)}</span>
		</div>
	{/if}
{/snippet}

{#snippet doneCard()}
	{#if run !== null}
		{@const words = doneWords(run, now)}
		<div
			class="done"
			class:undone={run.state === 'undone' || run.kind === 'undo'}
			data-testid="done"
		>
			<span class="kicker mono">{words.kicker}</span>
			<span class="done-title">{words.title}<span class="dot">.</span></span>
			{#if run.kind === 'run'}
				<span class="done-summary mono">{summaryWords(run).toUpperCase()}</span>
			{/if}
			<span class="done-body">{words.body}</span>
			{#if o.left.length > 0}
				<ul class="left-list">
					{#each o.left.slice(0, 20) as song (song.track)}
						<li class="mono">
							<span class="left-title">{song.title === '' ? song.from : song.title}</span>
							<span class="left-why">{reasonWords[song.reason]}</span>
						</li>
					{/each}
				</ul>
			{/if}
			{#if undoing === run.id}
				<div class="ask">
					<span class="ask-words">{undoAsk(run)}</span>
					<span class="mono note">ANY SONG MOVED SINCE STAYS PUT, AND IS NAMED AFTER</span>
					<button
						type="button"
						class="outline"
						onclick={() => {
							undoing = '';
						}}>Keep them</button
					>
					<button
						type="button"
						class="fox"
						onclick={() => {
							undoing = '';
							void o.undo(run.id);
						}}>Put them back</button
					>
				</div>
			{:else if run.undoable && can}
				<div class="undo-row">
					<button
						type="button"
						class="strong-outline"
						onclick={() => {
							undoing = run.id;
						}}>Undo this run</button
					>
					<span class="mono note">UNTIL A SONG IT MOVED MOVES AGAIN · KEPT 90 DAYS</span>
				</div>
			{/if}
		</div>
	{/if}
{/snippet}

{#snippet runsCard()}
	{#if o.runs.length === 0}
		<div class="runs empty">
			<span class="label mono">RUNS</span>
			<span class="runs-none"
				>No runs yet. Preview shows every move first; nothing moves until you press Run it, and each
				run can be undone until its songs move again.</span
			>
		</div>
	{:else}
		<div class="runs">
			<span class="label mono runs-label">RUNS</span>
			{#each o.runs as each (each.id)}
				{@const row = runRowWords(each)}
				<div class="run-row">
					<span class="mono when">{whenWords(each.startedAt, now)}</span>
					<span class="what">{row.what}</span>
					{#if each.undoable && can && each.id !== run?.id}
						<button
							type="button"
							class="run-state mono hot"
							onclick={() => {
								o.run = each;
								o.view = 'done';
								undoing = each.id;
							}}>{row.state}</button
						>
					{:else if each.undoable && can}
						<button
							type="button"
							class="run-state mono hot"
							onclick={() => {
								undoing = each.id;
							}}>{row.state}</button
						>
					{:else}
						<span class="run-state mono">{row.state}</span>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
{/snippet}

<style>
	.organize {
		--left: calc(480px * var(--step));
	}

	/* 4K: the board's wider template column, with the page's size step. */
	@media (min-width: 2560px) {
		.organize {
			--left: calc(640px * var(--step));
		}
	}

	.gone {
		padding: calc(var(--frame-top) + 40px) var(--gutter);
		color: var(--text-secondary);
	}

	.mono {
		font-family: var(--font-mono);
	}

	a {
		color: var(--fox-text);
	}

	.intro {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: 24px;
		margin-bottom: calc(40px * var(--step));
	}

	.intro-words {
		display: flex;
		flex-direction: column;
		gap: 10px;
		min-width: 0;
	}

	h1 {
		margin: 0;
		font-size: calc(72px * var(--step));
		font-weight: 900;
		letter-spacing: -0.06em;
		line-height: 1;
	}

	.dot {
		color: var(--fox);
	}

	.counts {
		font-size: calc(12px * var(--step));
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.promise {
		max-width: calc(360px * var(--step));
		margin-bottom: 4px;
		text-align: right;
		font-size: calc(11px * var(--step));
		line-height: 1.6;
		letter-spacing: 0.06em;
		color: var(--text-meta);
	}

	.body {
		display: flex;
		gap: calc(32px * var(--step));
		align-items: flex-start;
	}

	.left {
		position: sticky;
		top: calc(var(--frame-top) * var(--step));
		width: var(--left);
		flex: none;
		display: flex;
		flex-direction: column;
		gap: calc(14px * var(--step));
	}

	.right {
		position: relative;
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: calc(14px * var(--step));
	}

	.label-row {
		position: relative;
		display: flex;
		align-items: center;
		justify-content: space-between;
	}

	.label {
		font-size: calc(11px * var(--step));
		letter-spacing: 0.1em;
		color: var(--text-meta);
	}

	.spaced {
		margin-top: 4px;
	}

	button {
		cursor: pointer;
		font-family: var(--font-sans);
	}

	button:disabled {
		cursor: not-allowed;
		opacity: 0.5;
	}

	.presets {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		height: calc(32px * var(--step));
		padding: 0 12px;
		border-radius: var(--radius-pill);
		border: 1px solid var(--edge-strong);
		background: none;
		color: var(--text);
		font-size: calc(13px * var(--step));
		font-weight: 600;
	}

	.presets[aria-expanded='true'] {
		background: var(--hover);
	}

	.preset-menu {
		position: absolute;
		right: 0;
		top: calc(40px * var(--step));
		z-index: 6;
		width: calc(380px * var(--step));
		max-width: calc(100vw - 32px);
		padding: 6px;
		border-radius: var(--radius-16);
		background: var(--card-raised);
		border: 1px solid var(--edge-raised);
		box-shadow: var(--shadow-float);
	}

	.preset {
		width: 100%;
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: 3px;
		padding: 10px 12px;
		border: 0;
		border-radius: 10px;
		background: none;
		color: var(--text);
		text-align: left;
	}

	.preset:hover,
	.preset.current {
		background: color-mix(in srgb, var(--fox) 14%, transparent);
	}

	.preset-name {
		font-size: calc(14px * var(--step));
		font-weight: 600;
	}

	.preset-line {
		font-size: calc(11px * var(--step));
		color: var(--text-meta);
	}

	.hint,
	.fault {
		margin-top: -4px;
		font-size: calc(10.5px * var(--step));
		letter-spacing: 0.06em;
		color: var(--text-meta);
	}

	.fault {
		color: var(--problem);
	}

	.fold,
	.text-button {
		align-self: flex-start;
		padding: 4px 0;
		border: 0;
		background: none;
		color: var(--fox-text);
		font-size: calc(13px * var(--step));
		font-weight: 600;
	}

	.again {
		display: flex;
		gap: 18px;
	}

	.text-button.quiet {
		color: var(--text-secondary);
	}

	.folder-card {
		display: flex;
		flex-direction: column;
		gap: 10px;
		padding: 14px;
		border-radius: var(--radius-16);
		border: 1.5px dashed var(--edge-strong);
	}

	.folder-pick {
		display: flex;
		align-items: center;
		gap: 10px;
	}

	.folder-pick select {
		flex: 1;
		height: 32px;
		border-radius: var(--radius-12);
		border: 1px solid var(--edge-field);
		background: var(--input-solid);
		color: var(--text);
		font: 500 13px var(--font-mono);
	}

	.folder-buttons {
		display: flex;
		align-items: center;
		gap: 12px;
	}

	.rule {
		height: 1px;
		margin: 2px 0;
		background: var(--edge);
	}

	.options {
		display: flex;
		flex-direction: column;
		gap: 14px;
	}

	.buttons {
		display: flex;
		gap: 10px;
		margin-top: 6px;
	}

	.fox {
		height: calc(44px * var(--step));
		padding: 0 24px;
		border: 0;
		border-radius: var(--radius-pill);
		background: var(--fox);
		color: var(--fox-ink);
		font-size: calc(15px * var(--step));
		font-weight: 700;
	}

	.outline {
		height: calc(44px * var(--step));
		padding: 0 20px;
		border-radius: var(--radius-pill);
		border: 1px solid var(--edge-strong);
		background: none;
		color: var(--text);
		font-size: calc(15px * var(--step));
		font-weight: 600;
	}

	.outline:hover:not(:disabled) {
		background: var(--hover);
	}

	.outline.small {
		height: calc(36px * var(--step));
		font-size: calc(13px * var(--step));
	}

	.strong-outline {
		height: calc(44px * var(--step));
		padding: 0 22px;
		border-radius: var(--radius-pill);
		border: 1.5px solid var(--text);
		background: none;
		color: var(--text);
		font-size: calc(15px * var(--step));
		font-weight: 700;
	}

	.problem {
		color: var(--problem);
		font-size: var(--type-body);
	}

	.summary {
		display: flex;
		flex-direction: column;
		gap: 6px;
		padding: 16px 18px;
		border-radius: var(--radius-16);
		border: 1px solid var(--edge);
	}

	.summary-text {
		margin-bottom: 8px;
		font-size: calc(12.5px * var(--step));
		color: var(--text-secondary);
		word-break: break-all;
	}

	.scope {
		display: flex;
		align-items: center;
		gap: 16px;
		flex-wrap: wrap;
		font-size: calc(16px * var(--step));
		font-weight: 700;
	}

	.filters {
		display: flex;
		align-items: center;
		gap: 8px;
		flex-wrap: wrap;
	}

	.chips {
		display: flex;
		gap: 8px;
		flex-wrap: wrap;
	}

	.filter {
		flex: none;
		height: calc(34px * var(--step));
		padding: 0 12px;
		border-radius: var(--radius-pill);
		border: 1px solid var(--edge-strong);
		background: transparent;
		color: var(--text-secondary);
		font-size: calc(13px * var(--step));
		font-weight: 600;
	}

	.filter[aria-pressed='true'] {
		border-color: var(--fox-line);
		background: color-mix(in srgb, var(--fox) 12%, transparent);
		color: var(--text);
	}

	.count {
		font-size: calc(11px * var(--step));
		color: var(--text-meta);
	}

	.filter.hot .count,
	.filter[aria-pressed='true'] .count {
		color: var(--fox-text);
	}

	.search {
		margin-left: auto;
		display: flex;
		align-items: center;
		gap: 8px;
		width: calc(220px * var(--step));
		height: calc(34px * var(--step));
		box-sizing: border-box;
		padding: 0 12px;
		border-radius: var(--radius-pill);
		background: var(--control-wash);
		color: var(--text-meta);
	}

	.search input {
		flex: 1;
		min-width: 0;
		border: 0;
		background: transparent;
		color: var(--text);
		font-size: calc(13px * var(--step));
		outline: none;
	}

	.search input::placeholder {
		font-style: italic;
		color: var(--text-meta);
	}

	.search:focus-within {
		outline: 2px solid var(--fox-line);
	}

	.list {
		display: flex;
		flex-direction: column;
		gap: calc(10px * var(--step));
	}

	.list.docked {
		padding-bottom: calc(92px * var(--step));
	}

	.none {
		padding: 40px 0;
		text-align: center;
		color: var(--text-meta);
	}

	.sentinel {
		height: 1px;
	}

	.dock {
		position: sticky;
		bottom: calc(24px * var(--step));
		z-index: 2;
		margin-top: calc(-92px * var(--step));
		display: flex;
		align-items: center;
		gap: 14px;
		padding: 12px 12px 12px 22px;
		border-radius: var(--radius-pill);
		background: var(--glass-fill);
		backdrop-filter: var(--glass-blur);
		border: 1px solid var(--edge);
		box-shadow: var(--shadow-float);
	}

	.dock-words {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.dock-title {
		font-size: calc(15px * var(--step));
		font-weight: 700;
	}

	.dock-line {
		font-size: calc(10.5px * var(--step));
		letter-spacing: 0.06em;
		color: var(--text-meta);
	}

	.dock .fox {
		padding: 0 26px;
	}

	.head-card {
		position: sticky;
		top: calc(var(--frame-top) * var(--step));
		z-index: 2;
		display: flex;
		flex-direction: column;
		gap: 10px;
		padding: 18px 22px;
		border-radius: var(--radius-20);
		background: var(--card);
		border: 1.5px solid var(--fox-line);
	}

	.head-row {
		display: flex;
		align-items: baseline;
		gap: 14px;
	}

	.head-title {
		font-size: calc(26px * var(--step));
		font-weight: 900;
		letter-spacing: -0.03em;
	}

	.pace,
	.now {
		font-size: calc(11px * var(--step));
		letter-spacing: 0.06em;
		color: var(--text-meta);
	}

	.stop {
		margin-left: auto;
		height: calc(40px * var(--step));
		font-size: calc(14px * var(--step));
		font-weight: 700;
	}

	.bar {
		height: 8px;
		border-radius: 4px;
		background: var(--control-wash);
		overflow: hidden;
	}

	.bar span {
		display: block;
		height: 100%;
		background: var(--fox);
		transition: width 400ms var(--curve-settle);
	}

	.done {
		display: flex;
		flex-direction: column;
		gap: 14px;
		padding: calc(26px * var(--step)) calc(28px * var(--step));
		border-radius: var(--radius-24);
		background: color-mix(in srgb, var(--fox) 12%, transparent);
		border: 1.5px solid var(--fox-line);
	}

	.done.undone {
		background: var(--card);
		border-color: var(--edge-strong);
	}

	.kicker {
		font-size: calc(11px * var(--step));
		letter-spacing: 0.1em;
		color: var(--text-meta);
	}

	.done-title {
		font-size: calc(40px * var(--step));
		font-weight: 900;
		letter-spacing: -0.04em;
		line-height: 1.05;
	}

	.done-summary {
		font-size: calc(11px * var(--step));
		letter-spacing: 0.06em;
		color: var(--text-secondary);
	}

	.done-body {
		font-size: calc(15px * var(--step));
		line-height: 1.5;
		color: var(--text-secondary);
	}

	.left-list {
		margin: 0;
		padding: 0;
		list-style: none;
		display: flex;
		flex-direction: column;
		gap: 4px;
		font-size: calc(12px * var(--step));
	}

	.left-list li {
		display: flex;
		gap: 10px;
		justify-content: space-between;
	}

	.left-why {
		color: var(--text-meta);
		letter-spacing: 0.04em;
	}

	.ask {
		display: flex;
		align-items: center;
		gap: 12px;
		flex-wrap: wrap;
		padding: 14px 16px;
		border-radius: var(--radius-16);
		border: 1.5px dashed color-mix(in srgb, var(--fox-line) 55%, transparent);
		background: color-mix(in srgb, var(--fox) 12%, transparent);
	}

	.ask-words {
		font-size: calc(15px * var(--step));
		font-weight: 600;
	}

	.ask .outline {
		margin-left: auto;
		height: calc(40px * var(--step));
		font-size: calc(14px * var(--step));
	}

	.ask .fox {
		height: calc(40px * var(--step));
		font-size: calc(14px * var(--step));
	}

	.note {
		font-size: calc(10.5px * var(--step));
		letter-spacing: 0.06em;
		color: var(--text-meta);
	}

	.undo-row {
		display: flex;
		align-items: center;
		gap: 14px;
		flex-wrap: wrap;
	}

	.runs {
		display: flex;
		flex-direction: column;
		padding: 10px calc(24px * var(--step));
		border-radius: var(--radius-20);
		background: var(--card);
		border: 1px solid var(--edge);
	}

	.runs.empty {
		gap: 6px;
		padding: 18px calc(24px * var(--step));
		background: none;
		border: 1.5px dashed var(--edge-strong);
	}

	.runs-label {
		padding: 10px 0;
	}

	.runs-none {
		font-size: calc(15px * var(--step));
		color: var(--text-secondary);
	}

	.run-row {
		display: grid;
		grid-template-columns: calc(140px * var(--step)) minmax(0, 1fr) auto;
		align-items: center;
		gap: 14px;
		padding: 12px 0;
		border-top: 1px solid var(--edge);
	}

	.when {
		font-size: calc(11px * var(--step));
		color: var(--text-meta);
	}

	.what {
		font-size: calc(14px * var(--step));
		font-weight: 600;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.run-state {
		border: 0;
		background: none;
		font-size: calc(11px * var(--step));
		font-weight: 700;
		color: var(--text-meta);
	}

	.run-state.hot {
		color: var(--fox-text);
	}

	/* A phone: one column, the filters sliding sideways, Run it the width of the page. */
	.phone .intro {
		margin-bottom: 14px;
	}

	.phone h1 {
		font-size: 48px;
	}

	.phone .counts {
		font-size: 10.5px;
		line-height: 1.5;
	}

	.phone .body {
		flex-direction: column;
		gap: 16px;
	}

	.phone .left {
		position: static;
		width: 100%;
	}

	.phone .right {
		width: 100%;
	}

	.phone .filters {
		flex-wrap: nowrap;
		flex-direction: column;
		align-items: stretch;
	}

	.phone .chips {
		flex-wrap: nowrap;
		overflow-x: auto;
		margin: 0 calc(-1 * var(--gutter));
		padding: 0 var(--gutter);
		scrollbar-width: none;
	}

	.phone .search {
		margin-left: 0;
		width: 100%;
	}

	.dock.phone {
		position: sticky;
		bottom: 0;
		flex-direction: column;
		align-items: stretch;
		gap: 8px;
		margin: calc(-92px * var(--step)) calc(-1 * var(--gutter)) 0;
		padding: 12px var(--gutter) calc(26px + env(safe-area-inset-bottom));
		border: 0;
		border-radius: 0;
		box-shadow: none;
		background: linear-gradient(to top, var(--ground) 70%, transparent);
		backdrop-filter: none;
	}

	.fox.wide {
		width: 100%;
		height: 52px;
		font-size: 16px;
	}

	.dock.phone .dock-line {
		text-align: center;
		font-size: 10px;
	}

	.phone .head-card {
		top: 72px;
		padding: 14px 16px;
	}

	.phone .head-title {
		font-size: 20px;
	}

	.phone .done-title {
		font-size: 30px;
	}

	.phone .strong-outline {
		width: 100%;
	}

	.phone .run-row {
		grid-template-columns: minmax(0, 1fr) auto;
	}

	.phone .when {
		grid-column: 1 / -1;
	}

	@media (prefers-reduced-motion: reduce) {
		.bar span {
			transition: none;
		}
	}
</style>
