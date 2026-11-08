<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { resolve } from '$app/paths';
	import { library, type Load } from '#lib/library/library.svelte.ts';
	import { SettingsError, type ServerSettings } from '#lib/library/source.ts';
	import ScanCard from '#lib/pages/ScanCard.svelte';
	import FolderPicker from '#lib/pages/FolderPicker.svelte';
	import SettingsSection from '#lib/pages/SettingsSection.svelte';
	import {
		afterSave,
		changeFor,
		dirty,
		draftOf,
		folderLine,
		fromLine,
		lastScanLine,
		levels,
		libraryFactsLine,
		restartNote,
		sectionOfKey,
		undone,
		type Draft,
		type SectionId
	} from '#lib/pages/serverForm.ts';

	/** How long a section says SAVED. */
	const savedMs = 2400;
	/** How often the words that depend on the clock are worked out again. */
	const tickMs = 1000;

	let load = $state.raw<Load<ServerSettings>>({ kind: 'loading' });
	let base = $state.raw<Draft | null>(null);
	let draft = $state<Draft | null>(null);
	let saving = $state<SectionId | null>(null);
	let saved = $state<SectionId | null>(null);
	let problems = $state<Record<SectionId, string>>({
		library: '',
		lastfm: '',
		scanning: '',
		advanced: ''
	});
	let scanTrouble = $state('');
	let now = $state(Date.now());
	let apiKeyField = $state<HTMLInputElement | undefined>();
	let secretField = $state<HTMLInputElement | undefined>();
	let picking = $state(false);

	const facts = $derived(library.facts);
	const settings = $derived(load.kind === 'ready' ? load.value : null);
	const scanning = $derived(facts?.scan.state === 'running');
	const pass = $derived(facts?.fingerprints);
	const passing = $derived(pass !== undefined && pass.done < pass.total);
	const graceHint = $derived(
		'A song that disappears keeps its plays, hearts and ratings this long, in case its folder comes back.' +
			(!passing && facts !== null && facts.tracks > 0
				? ' Every song has its fingerprint, so a moved file keeps them too.'
				: '')
	);
	const bytes = $derived(settings?.folders.reduce((sum, folder) => sum + folder.bytes, 0) ?? 0);

	function changed(section: SectionId): boolean {
		return base !== null && draft !== null && dirty(base, draft, section);
	}

	async function read(): Promise<void> {
		try {
			const value = await library.serverSettings();
			load = { kind: 'ready', value };
			base = draftOf(value);
			draft = draftOf(value);
		} catch {
			load = { kind: 'unavailable' };
		}
	}

	onMount(() => {
		void library.refreshFacts();
		void read().then(async () => {
			if (location.hash === '#lastfm') {
				await tick();
				document.getElementById('lastfm')?.scrollIntoView({ block: 'center' });
				apiKeyField?.focus({ preventScroll: true });
			}
		});
		const clock = setInterval(() => {
			now = Date.now();
		}, tickMs);
		return () => {
			clearInterval(clock);
		};
	});

	let savedTimer: ReturnType<typeof setTimeout> | undefined;

	async function save(section: SectionId): Promise<void> {
		if (base === null || draft === null) {
			return;
		}
		problems[section] = '';
		const planned = changeFor(base, draft, section);
		if ('problem' in planned) {
			problems[sectionOfKey[planned.problem.key]] = planned.problem.message;
			return;
		}
		saving = section;
		try {
			const value = await library.changeSettings(planned.change);
			const next = draftOf(value);
			draft = afterSave(base, next, draft, section);
			base = next;
			load = { kind: 'ready', value };
			saved = section;
			clearTimeout(savedTimer);
			savedTimer = setTimeout(() => {
				saved = null;
			}, savedMs);
		} catch (error) {
			if (error instanceof SettingsError) {
				problems[error.key === undefined ? section : sectionOfKey[error.key]] =
					`That wasn’t saved: ${error.detail}.`;
			} else {
				problems[section] = 'That wasn’t saved: the server couldn’t be reached. Try again.';
			}
		} finally {
			saving = null;
		}
	}

	function undo(section: SectionId): void {
		if (base !== null && draft !== null) {
			draft = undone(base, draft, section);
			problems[section] = '';
		}
	}

	/** Adds a folder the picker chose, and saves at once: the scan it starts is the point. */
	function addRoot(path: string): void {
		if (draft !== null && !draft.roots.includes(path)) {
			draft.roots = [...draft.roots, path];
			void save('library');
		}
	}

	function removeRoot(root: string): void {
		if (draft !== null) {
			draft.roots = draft.roots.filter((each) => each !== root);
		}
	}

	async function replaceSecret(): Promise<void> {
		if (draft !== null) {
			draft.secret = { kind: 'replace', text: '' };
			await tick();
			secretField?.focus();
		}
	}

	async function scanNow(): Promise<void> {
		scanTrouble = '';
		const refusal = await library.startScan('quick');
		if (refusal !== null) {
			scanTrouble = 'The scan didn’t start. Try again in a moment.';
		}
	}

	function folderOf(path: string): { tracks: number; bytes: number } | undefined {
		return settings?.folders.find((folder) => folder.path === path);
	}
</script>

<svelte:head>
	<title>Server · Kitsune</title>
</svelte:head>

<div class="server">
	<div class="intro" data-morph-part="intro" data-morph-key="intro">
		<a class="crumb mono" href={resolve('/you')}>YOU › SERVER</a>
		<h1>Server<span class="dot">.</span></h1>
		<p class="lede">Everything this server does, set here. Only administrators see this page.</p>
		{#if facts !== null}
			<p class="mono compact">{libraryFactsLine(facts, bytes).toUpperCase()}</p>
		{/if}
		<dl>
			{#if facts !== null}
				<dt class="mono">LIBRARY</dt>
				<dd>{libraryFactsLine(facts, bytes)}</dd>
				<dt class="mono">LAST SCAN</dt>
				<dd>{lastScanLine(facts.scan.last, scanning, now)}</dd>
			{/if}
			{#if settings !== null}
				<dt class="mono">VERSION</dt>
				<dd class="code">{settings.version}</dd>
				<dt class="mono">LISTENING ON</dt>
				<dd class="code">
					{settings.listen}{settings.listenSource === 'server.toml' ? ' · from server.toml' : ''}
				</dd>
			{/if}
		</dl>
	</div>

	{#if !library.admin}
		<p class="note">Only administrators see this page. <a href={resolve('/you')}>Back to You</a></p>
	{:else if load.kind === 'unavailable'}
		<p class="note" role="status">
			The settings couldn’t be read.
			<button type="button" class="link" onclick={() => void read()}>Try again</button>
		</p>
	{:else if settings !== null && draft !== null}
		{@const s = settings.settings}
		<div class="sections">
			<SettingsSection
				id="library"
				title="Library"
				changed={changed('library')}
				saving={saving === 'library'}
				saved={saved === 'library'}
				problem={problems.library}
				onundo={() => {
					undo('library');
				}}
				onsave={() => void save('library')}
			>
				<div class="folders">
					{#each draft.roots as root (root)}
						{@const held = folderOf(root)}
						<div class="folder">
							<svg
								class="folder-icon"
								width="20"
								height="20"
								viewBox="0 0 24 24"
								aria-hidden="true"
							>
								<path
									d="M3 6.5A1.5 1.5 0 0 1 4.5 5h4.6l2 2.2h8.4A1.5 1.5 0 0 1 21 8.7v9.8a1.5 1.5 0 0 1-1.5 1.5h-15A1.5 1.5 0 0 1 3 18.5z"
								></path>
							</svg>
							<span class="folder-words">
								<span class="path">{root}</span>
								<span class="sub">
									{folderLine(
										held?.tracks ?? 0,
										held?.bytes ?? 0,
										base?.roots.includes(root) ?? false
									)}
								</span>
							</span>
							<button
								type="button"
								class="remove"
								aria-label={`Remove ${root}`}
								onclick={() => {
									removeRoot(root);
								}}
							>
								<svg width="16" height="16" viewBox="0 0 24 24" aria-hidden="true">
									<path d="M6 6l12 12M18 6L6 18"></path>
								</svg>
							</button>
						</div>
					{:else}
						<p class="empty">No folders yet. Add the one your music is in, and a scan reads it.</p>
					{/each}
					<button
						type="button"
						class="add-folder"
						disabled={saving === 'library'}
						onclick={() => {
							picking = true;
						}}>Add a folder</button
					>
					{#if base !== null && base.roots.some((root) => !draft?.roots.includes(root))}
						<p class="hint">
							A removed folder’s songs go only once a scan has read every folder left, so a disk
							that’s offline never looks like deleted music.
						</p>
					{/if}
				</div>
				{@const from = fromLine(s['library.roots'])}
				{#if from !== null}
					<span class="from mono">{from}</span>
				{/if}
				<label class="switch">
					<input type="checkbox" bind:checked={draft.tagWrites} />
					<span class="track" aria-hidden="true"><span class="knob"></span></span>
					<span class="words">
						<span class="name">Let Kitsune write tags to your files</span>
						<span class="hint"
							>Off: editing tags is turned off and nothing in your folders ever changes. On: the tag
							editor saves into the files.</span
						>
						{#if fromLine(s['library.tag_writes']) !== null}
							<span class="from mono">{fromLine(s['library.tag_writes'])}</span>
						{/if}
					</span>
				</label>
			</SettingsSection>

			<SettingsSection
				id="lastfm"
				title="Last.fm"
				changed={changed('lastfm')}
				saving={saving === 'lastfm'}
				saved={saved === 'lastfm'}
				problem={problems.lastfm}
				onundo={() => {
					undo('lastfm');
				}}
				onsave={() => void save('lastfm')}
			>
				<p class="lede small">
					With a key, everyone here can connect their own Last.fm account on their You page.
					<a href="https://www.last.fm/api/account/create" target="_blank" rel="noopener noreferrer"
						>Get a key from Last.fm</a
					>
				</p>
				<label class="field">
					<span class="label">API key</span>
					<input
						class="input code"
						bind:this={apiKeyField}
						bind:value={draft.apiKey}
						autocomplete="off"
						spellcheck="false"
						placeholder="32 letters and digits"
					/>
					{#if fromLine(s['lastfm.api_key']) !== null}
						<span class="from mono">{fromLine(s['lastfm.api_key'])}</span>
					{/if}
				</label>
				<div class="field">
					<span class="label" id="secret-label">Shared secret</span>
					{#if draft.secret.kind === 'replace' || (draft.secret.kind === 'keep' && !s['lastfm.shared_secret'].set)}
						<input
							class="input code"
							type="password"
							aria-labelledby="secret-label"
							bind:this={secretField}
							value={draft.secret.kind === 'replace' ? draft.secret.text : ''}
							oninput={(event) => {
								if (draft !== null) {
									draft.secret = { kind: 'replace', text: event.currentTarget.value };
								}
							}}
							autocomplete="off"
							placeholder="Paste the shared secret"
						/>
					{:else}
						<div class="secret">
							<span class="secret-state">
								{draft.secret.kind === 'clear' ? 'Cleared when you save' : 'Set · never shown'}
							</span>
							{#if draft.secret.kind === 'clear'}
								<button
									type="button"
									class="pill"
									onclick={() => {
										if (draft !== null) draft.secret = { kind: 'keep' };
									}}>Keep it</button
								>
							{:else}
								<button type="button" class="pill" onclick={() => void replaceSecret()}
									>Replace</button
								>
								<button
									type="button"
									class="pill quiet"
									onclick={() => {
										if (draft !== null) draft.secret = { kind: 'clear' };
									}}>Clear</button
								>
							{/if}
						</div>
					{/if}
					{#if fromLine(s['lastfm.shared_secret']) !== null}
						<span class="from mono">{fromLine(s['lastfm.shared_secret'])}</span>
					{/if}
				</div>
			</SettingsSection>

			<SettingsSection
				id="scanning"
				title="Scanning"
				changed={changed('scanning')}
				saving={saving === 'scanning'}
				saved={saved === 'scanning'}
				problem={problems.scanning || scanTrouble}
				onundo={() => {
					undo('scanning');
				}}
				onsave={() => void save('scanning')}
			>
				{#snippet action()}
					<button
						type="button"
						class="pill outline"
						disabled={scanning}
						onclick={() => void scanNow()}
					>
						{scanning ? 'Scanning…' : 'Scan now'}
					</button>
				{/snippet}
				{#if scanning && facts !== null}
					<ScanCard kind="scan" scan={facts.scan} {now} />
				{/if}
				<div class="pair">
					<label class="field">
						<span class="label">Look for changes every night at</span>
						<input class="input code" type="time" bind:value={draft.quickScan} required />
						{#if fromLine(s['library.quick_scan']) !== null}
							<span class="from mono">{fromLine(s['library.quick_scan'])}</span>
						{/if}
					</label>
					<label class="field">
						<span class="label">Forget missing songs after</span>
						<span class="days">
							<input class="input code short" inputmode="decimal" bind:value={draft.graceDays} />
							<span>{draft.graceDays.trim() === '1' ? 'day' : 'days'}</span>
						</span>
						{#if fromLine(s['library.missing_grace']) !== null}
							<span class="from mono">{fromLine(s['library.missing_grace'])}</span>
						{/if}
					</label>
				</div>
				{#if passing && pass !== undefined}
					<ScanCard kind="pass" {pass} />
				{/if}
				<p class="hint">
					{graceHint}
				</p>
			</SettingsSection>

			<SettingsSection
				id="advanced"
				title="Advanced"
				changed={changed('advanced')}
				saving={saving === 'advanced'}
				saved={saved === 'advanced'}
				problem={problems.advanced}
				onundo={() => {
					undo('advanced');
				}}
				onsave={() => void save('advanced')}
			>
				<label class="field">
					<span class="label row">
						<span>Public address</span>
						<span class="note-mono mono" class:pending={s['server.base_url'].pendingRestart}
							>{restartNote(s['server.base_url'])}</span
						>
					</span>
					<input
						class="input code"
						bind:value={draft.baseUrl}
						inputmode="url"
						spellcheck="false"
						placeholder="https://music.example.com"
					/>
					{#if fromLine(s['server.base_url']) !== null}
						<span class="from mono">{fromLine(s['server.base_url'])}</span>
					{/if}
				</label>
				<label class="field">
					<span class="label row">
						<span>Trusted proxies</span>
						<span class="note-mono mono" class:pending={s['server.trusted_proxies'].pendingRestart}
							>{restartNote(s['server.trusted_proxies'])}</span
						>
					</span>
					<input
						class="input code"
						bind:value={draft.proxies}
						spellcheck="false"
						placeholder="10.0.0.2, 172.17.0.0/16"
					/>
					{#if fromLine(s['server.trusted_proxies']) !== null}
						<span class="from mono">{fromLine(s['server.trusted_proxies'])}</span>
					{/if}
				</label>
				<div class="field">
					<span class="label" id="level-label">Log detail</span>
					<div class="levels" role="radiogroup" aria-labelledby="level-label">
						{#each levels as level (level.value)}
							<button
								type="button"
								role="radio"
								aria-checked={draft.level === level.value}
								class:on={draft.level === level.value}
								onclick={() => {
									if (draft !== null) draft.level = level.value;
								}}>{level.label}</button
							>
						{/each}
					</div>
					<select class="input level-select" aria-labelledby="level-label" bind:value={draft.level}>
						{#each levels as level (level.value)}
							<option value={level.value}>{level.label}</option>
						{/each}
					</select>
					{#if fromLine(s['log.level']) !== null}
						<span class="from mono">{fromLine(s['log.level'])}</span>
					{/if}
				</div>
			</SettingsSection>
		</div>
	{/if}
</div>

<FolderPicker
	open={picking}
	onclose={() => {
		picking = false;
	}}
	onadd={(path) => {
		picking = false;
		addRoot(path);
	}}
/>

<style>
	.server {
		--section-pad: 32px;
		--section-radius: var(--radius-28);
		--section-gap: 20px;
		--section-title: var(--type-section);
		--field-height: 52px;
		--field-font: 15px;
		--intro-width: 480px;
		--headline: var(--type-hero);

		box-sizing: border-box;
		max-width: 2560px;
		margin: 0 auto;
		padding: 128px 64px 200px;
		display: grid;
		grid-template-columns: var(--intro-width) minmax(0, 1fr);
		gap: 56px;
		align-items: start;
	}

	.mono {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		font-weight: 400;
		letter-spacing: 0.08em;
	}

	.code {
		font-family: var(--font-mono);
	}

	.crumb {
		color: var(--text-meta);
		text-decoration: none;
	}

	.crumb:hover {
		color: var(--fox-text);
	}

	h1 {
		margin-top: 20px;
		font-size: clamp(64px, calc(var(--intro-width) * 0.29), var(--headline));
		font-weight: 900;
		letter-spacing: var(--tracking-hero);
		line-height: 0.82;
	}

	.dot {
		color: var(--fox-text);
	}

	.lede {
		margin-top: 32px;
		max-width: 420px;
		font-size: 17px;
		line-height: 1.55;
		color: var(--text-secondary);
	}

	.lede.small {
		margin: 0;
		max-width: none;
		font-size: 14px;
	}

	a {
		color: var(--fox-text);
	}

	.compact {
		display: none;
		color: var(--text-meta);
	}

	dl {
		margin-top: 48px;
		display: grid;
		grid-template-columns: 120px 1fr;
		row-gap: 14px;
		font-size: 14px;
	}

	dt {
		padding-top: 2px;
		color: var(--text-meta);
	}

	dd.code {
		font-size: 13px;
	}

	.note {
		align-self: center;
		font-size: var(--type-body);
		color: var(--text-secondary);
	}

	.link {
		border: 0;
		background: none;
		color: var(--fox-text);
		font: inherit;
		text-decoration: underline;
		cursor: pointer;
	}

	.sections {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 32px;
		align-items: start;
	}

	.folders {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.folder {
		display: flex;
		align-items: center;
		gap: 14px;
		min-height: 60px;
		padding: 6px 8px 6px 18px;
		border-radius: var(--radius-16);
		background: var(--input);
	}

	.folder-icon path {
		fill: none;
		stroke: var(--text-secondary);
		stroke-width: 1.8;
	}

	.folder-words {
		flex: 1;
		min-width: 0;
	}

	.path {
		display: block;
		font-family: var(--font-mono);
		font-size: 14px;
		overflow-wrap: anywhere;
	}

	.sub {
		display: block;
		margin-top: 2px;
		font-size: 12px;
		color: var(--text-meta);
	}

	.remove {
		flex: none;
		width: var(--target);
		height: var(--target);
		display: grid;
		place-items: center;
		border: 0;
		border-radius: var(--radius-pill);
		background: transparent;
		cursor: pointer;
	}

	.remove path {
		stroke: var(--text-meta);
		stroke-width: 2;
		stroke-linecap: round;
	}

	.remove:hover {
		background: var(--hover);
	}

	.add-folder {
		height: 52px;
		border: 1px dashed var(--edge-strong);
		border-radius: var(--radius-16);
		background: transparent;
		color: var(--text);
		font: inherit;
		font-size: 14px;
		font-weight: 600;
		cursor: pointer;
		transition:
			border-color var(--morph-fast) var(--curve-morph),
			background var(--morph-fast) var(--curve-morph);
	}

	.add-folder:hover:not(:disabled) {
		border-color: var(--fox-line);
		background: var(--hover);
	}

	.empty,
	.hint {
		font-size: 13px;
		line-height: 1.5;
		color: var(--text-meta);
	}

	.from {
		font-size: 11px;
		letter-spacing: 0.06em;
		color: var(--text-meta);
	}

	/* A real checkbox, drawn as a switch: it keeps the keyboard and the label. */
	.switch {
		position: relative;
		display: flex;
		align-items: flex-start;
		gap: 14px;
		cursor: pointer;
	}

	.switch input {
		position: absolute;
		width: 1px;
		height: 1px;
		opacity: 0;
		margin: 0;
	}

	.track {
		position: relative;
		flex: none;
		width: 44px;
		height: 26px;
		margin-top: 1px;
		border-radius: var(--radius-pill);
		background: var(--rail);
		transition: background var(--morph-fast) var(--curve-morph);
	}

	.knob {
		position: absolute;
		top: 3px;
		left: 3px;
		width: 20px;
		height: 20px;
		border-radius: 50%;
		background: var(--text-secondary);
		transition:
			transform var(--morph-fast) var(--curve-settle),
			background var(--morph-fast) var(--curve-morph);
	}

	.switch input:checked + .track {
		background: var(--fox);
	}

	.switch input:checked + .track .knob {
		transform: translateX(18px);
		background: var(--fox-ink);
	}

	.switch input:focus-visible + .track {
		outline: 2px solid var(--fox-line);
		outline-offset: 2px;
	}

	.words {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.name {
		font-size: var(--type-body);
		font-weight: 600;
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: 8px;
		min-width: 0;
	}

	.label {
		font-size: 14px;
		font-weight: 600;
		color: var(--text-secondary);
	}

	.label.row {
		display: flex;
		justify-content: space-between;
		gap: 12px;
	}

	.note-mono {
		font-size: 11px;
		letter-spacing: 0.06em;
		color: var(--text-meta);
	}

	.note-mono.pending {
		color: var(--fox-text);
	}

	.input {
		box-sizing: border-box;
		width: 100%;
		height: var(--field-height);
		padding: 0 20px;
		border: 1px solid var(--edge-field);
		border-radius: var(--radius-pill);
		background: var(--input);
		color: var(--text);
		font-size: var(--field-font);
		outline: none;
		transition: border-color var(--morph-fast) var(--curve-morph);
	}

	.input:hover {
		border-color: var(--edge-hover);
	}

	.input:focus-visible {
		border-color: var(--fox-line);
	}

	.input::placeholder {
		color: var(--text-label);
	}

	.input.short {
		width: 96px;
	}

	.days {
		display: flex;
		align-items: center;
		gap: 10px;
		font-size: 14px;
		color: var(--text-secondary);
	}

	.pair {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 16px;
		align-items: start;
	}

	.secret {
		display: flex;
		align-items: center;
		gap: 8px;
		min-height: var(--field-height);
		box-sizing: border-box;
		padding: 0 6px 0 20px;
		border: 1px solid var(--edge);
		border-radius: var(--radius-pill);
	}

	.secret-state {
		flex: 1;
		font-size: 14px;
		color: var(--text-secondary);
	}

	.pill {
		height: 40px;
		padding: 0 16px;
		border: 0;
		border-radius: var(--radius-pill);
		background: var(--fill);
		color: var(--text);
		font: inherit;
		font-size: 13px;
		font-weight: 600;
		cursor: pointer;
	}

	.pill:hover:not(:disabled) {
		background: var(--pressed);
	}

	.pill.quiet {
		background: transparent;
		color: var(--text-secondary);
	}

	.pill.outline {
		height: var(--target);
		padding: 0 18px;
		border: 1px solid var(--edge-strong);
		background: transparent;
		font-size: 14px;
	}

	.pill:disabled {
		opacity: 0.6;
		cursor: default;
	}

	.levels {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		width: fit-content;
		padding: 4px;
		border-radius: var(--radius-pill);
		background: var(--input);
		border: 1px solid var(--edge);
	}

	.levels button {
		height: 40px;
		padding: 0 16px;
		border: 0;
		border-radius: var(--radius-pill);
		background: transparent;
		color: var(--text-secondary);
		font: inherit;
		font-size: 13px;
		font-weight: 600;
		cursor: pointer;
	}

	.levels button.on {
		background: var(--fox);
		color: var(--fox-ink);
		font-weight: 700;
	}

	.levels button:focus-visible {
		outline: 2px solid var(--fox-line);
		outline-offset: 2px;
	}

	.level-select {
		display: none;
	}

	/* At 4K the four sections sit side by side, one step larger. */
	@media (min-width: 3000px) {
		.server {
			--section-pad: 40px;
			--section-radius: var(--radius-32);
			--section-gap: 24px;
			--section-title: 52px;
			--field-height: 64px;
			--field-font: 18px;
			--intro-width: 880px;
			--headline: 260px;

			max-width: none;
			padding: 200px 96px 240px;
			gap: 64px;
			font-size: 18px;
		}

		.sections {
			grid-template-columns: repeat(4, minmax(0, 1fr));
			gap: 40px;
		}

		.lede {
			margin-top: 44px;
			max-width: 560px;
			font-size: 21px;
		}

		.lede.small,
		.label,
		.secret-state,
		.days {
			font-size: 17px;
		}

		.hint,
		.empty {
			font-size: 16px;
		}

		.mono,
		dt {
			font-size: 15px;
		}

		dl {
			grid-template-columns: 170px 1fr;
			row-gap: 18px;
			font-size: 18px;
		}

		.pair {
			grid-template-columns: minmax(0, 1fr);
		}
	}

	@media (max-width: 1500px) {
		.server {
			grid-template-columns: minmax(0, 1fr);
			padding-top: 112px;
		}

		.intro dl {
			grid-template-columns: 120px 1fr;
		}
	}

	@media (max-width: 1000px) {
		.sections {
			grid-template-columns: minmax(0, 1fr);
			gap: 16px;
		}
	}

	@media (max-width: 720px) {
		.server {
			--section-pad: 22px;
			--section-radius: var(--radius-24);
			--section-gap: 14px;
			--section-title: 28px;
			--field-height: 48px;
			--field-font: 14px;
			--headline: 80px;

			padding: 96px 16px 180px;
			gap: 16px;
		}

		h1 {
			margin-top: 16px;
			line-height: 0.85;
		}

		.lede,
		dl {
			display: none;
		}

		.compact {
			display: block;
			margin-top: 16px;
		}

		.pair {
			grid-template-columns: minmax(0, 1fr);
		}

		.levels {
			display: none;
		}

		.level-select {
			display: block;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.track,
		.knob {
			transition-duration: 120ms;
		}
	}
</style>
