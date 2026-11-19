<!--
	The rules editor, in place of a smart playlist's head and songs: never a modal. A 72px mosaic of
	what the draft chooses, the name at 48px, then two cards: the rules on the left (all or any, a row
	each, one level of group, the limit and the order, and the dock with Leave it and Save rules), and
	what they choose on the right, asked of the server as they change. Esc is Leave it; so is Back,
	through the history entry the page gives it. On a phone the count rides in a header that stays put
	while the rules scroll under it, each rule is a card, the limit folds into a pill, and the songs
	are listed under the rules.
-->
<script lang="ts">
	import { getContext, onDestroy, onMount, untrack } from 'svelte';
	import { morphsKey } from '#lib/morph/engine.ts';
	import type { Morphs } from '#lib/morph/engines/spring.ts';
	import { pressedPoint, Surface } from '#lib/morph/surface.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import { choosers, orders, ruleCount } from '#lib/library/smartlist.ts';
	import type {
		PlaylistAlbum,
		SmartOrder,
		SmartPreviewRequest,
		SmartRules
	} from '#lib/library/source.ts';
	import Mosaic from './Mosaic.svelte';
	import PillSelect from './PillSelect.svelte';
	import { dockLine, limitPill, matchWord } from './playlistWords.ts';
	import RuleRow from './RuleRow.svelte';
	import SmartLive from './SmartLive.svelte';
	import {
		isDraftGroup,
		type LivePreview,
		type RulePlace,
		type SmartDraft
	} from './smartRules.svelte.ts';

	interface Props {
		draft: SmartDraft;
		preview: LivePreview;
		/** What a preview asks with besides the rules: the playlist's ID, or a new draft's seed. */
		requestOf: (rules: SmartRules) => SmartPreviewRequest;
		/** The mosaic shown before the first answer. */
		mosaic: readonly PlaylistAlbum[];
		/** How many the saved rules choose, and which; undefined and null for a new draft. */
		savedCount: number | undefined;
		savedIds: ReadonlySet<string> | null;
		/** The playlists a Playlist rule may name. */
		playlists: readonly { id: string; name: string }[];
		saving: boolean;
		onleave: () => void;
		onsave: () => void;
		/**
		 * It opens over a page that was there (Edit rules), so it buds from the press, and closing lays
		 * the head and songs again in front of it. A new draft is a page of its own and does neither.
		 */
		buds?: boolean;
	}

	let {
		draft,
		preview,
		requestOf,
		mosaic,
		savedCount,
		savedIds,
		playlists,
		saving,
		onleave,
		onsave,
		buds = false
	}: Props = $props();

	const morphs = getContext<Morphs | undefined>(morphsKey);
	const surface = new Surface({ big: true, under: 'page' });
	let editor: HTMLElement | undefined = $state();

	onMount(() => {
		if (!untrack(() => buds) || editor === undefined) {
			return;
		}
		surface.open(editor, null);
		return () => {
			surface.leave();
			morphs?.layAgain(pressedPoint());
		};
	});

	let windowWidth = $state(1280);
	const phone = $derived(windowWidth < 700);
	const wide = $derived(windowWidth >= 2560);
	/** The phone's limit and order, folded into their pill until it's pressed. */
	let limitOpen = $state(false);

	const answer = $derived(preview.answer);
	const changed = $derived(draft.changed);
	const shownMosaic = $derived(answer?.mosaic ?? mosaic);
	const picks = $derived(playlists.map(({ id }) => id));

	onMount(() => {
		if (library.index.kind !== 'ready') {
			void library.loadIndex();
		}
		void preview.now(untrack(() => requestOf(draft.rules)));
	});

	let first = true;
	$effect(() => {
		const rules = draft.rules;
		if (first) {
			first = false;
			return;
		}
		untrack(() => {
			preview.request(requestOf(rules));
		});
	});

	onDestroy(() => {
		preview.stop();
	});

	function joinOf(at: number): string {
		return at === 0 ? 'WHERE' : draft.match === 'all' ? 'AND' : 'OR';
	}

	function place(at: number, inGroup?: number): RulePlace {
		return inGroup === undefined ? { at } : { at, in: inGroup };
	}

	const orderOptions = $derived(orders.filter(([v]) => draft.limit.on || v !== 'chosen'));

	function onkey(event: KeyboardEvent): void {
		if (event.key === 'Escape' && !event.defaultPrevented) {
			event.preventDefault();
			onleave();
		}
	}
</script>

<svelte:window bind:innerWidth={windowWidth} onkeydown={onkey} />

{#snippet ruleList()}
	<div class="match-line">
		<span>Songs where</span>
		<PillSelect
			value={draft.match}
			options={[
				['all', 'all'],
				['any', 'any']
			]}
			label="Match all or any"
			look="fox"
			onchange={(match) => {
				draft.match = match;
			}}
		/>
		<span>{phone ? 'are true' : 'of these are true'}</span>
	</div>

	{#each draft.items as item, at (item.key)}
		{#if isDraftGroup(item)}
			<div class="group-row">
				<span class="join fox">{joinOf(at)}</span>
				<div class="group">
					<div class="group-head">
						<PillSelect
							value={item.match}
							options={[
								['any', 'Any'],
								['all', 'All']
							]}
							label="This group matches all or any"
							look="fox"
							size="small"
							onchange={(match) => {
								draft.setGroupMatch(at, match);
							}}
						/>
						<span>of these</span>
						<span class="grow"></span>
						<button
							type="button"
							class="quiet"
							onclick={() => {
								draft.remove(place(at));
							}}>Take the group out</button
						>
					</div>
					{#each item.rules as { key, rule }, i (key)}
						<RuleRow
							{rule}
							join={i === 0 ? '' : item.match === 'all' ? 'AND' : 'OR'}
							joinFox
							inGroup
							{phone}
							{playlists}
							onfield={(field, choices) => {
								draft.setField(place(at, i), field, choices);
							}}
							onop={(op) => {
								draft.patch(place(at, i), { op });
							}}
							onvalue={(value) => {
								draft.patch(place(at, i), { value });
							}}
							onremove={() => {
								draft.remove(place(at, i));
							}}
						/>
					{/each}
					<button
						type="button"
						class="in-group-add"
						onclick={() => {
							draft.addToGroup(at);
						}}
						><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 5v14M5 12h14"></path></svg
						>Rule in this group</button
					>
				</div>
			</div>
		{:else}
			<RuleRow
				rule={item.rule}
				join={joinOf(at)}
				joinFox={at > 0}
				{phone}
				{playlists}
				onfield={(field, choices) => {
					draft.setField(place(at), field, choices);
				}}
				onop={(op) => {
					draft.patch(place(at), { op });
				}}
				onvalue={(value) => {
					draft.patch(place(at), { value });
				}}
				onremove={() => {
					draft.remove(place(at));
				}}
			/>
		{/if}
	{/each}

	<div class="adds">
		<button
			type="button"
			class="add fox"
			onclick={() => {
				draft.addRule(picks);
			}}
			><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 5v14M5 12h14"></path></svg
			>Rule</button
		>
		{#if draft.canGroup}
			<button
				type="button"
				class="add"
				onclick={() => {
					draft.addGroup();
				}}
				><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 5v14M5 12h14"></path></svg
				>Group</button
			>
		{/if}
		{#if phone}
			<button
				type="button"
				class="add pill"
				aria-expanded={limitOpen}
				onclick={() => {
					limitOpen = !limitOpen;
				}}>{limitPill(draft.limit)} ›</button
			>
		{/if}
	</div>

	{#if !phone || limitOpen}
		<div class="limits">
			<div class="limit-row" class:off={!draft.limit.on}>
				<button
					type="button"
					class="switch"
					role="switch"
					aria-checked={draft.limit.on}
					aria-label="Limit"
					onclick={() => {
						draft.toggleLimit();
					}}><span class="knob"></span></button
				>
				<span class="words">Keep at most</span>
				<input
					class="count"
					type="number"
					inputmode="numeric"
					min="1"
					value={draft.limit.count}
					aria-label="How many"
					disabled={!draft.limit.on}
					oninput={(event) => {
						const n = Math.trunc(Number(event.currentTarget.value));
						if (Number.isFinite(n) && n > 0) {
							draft.setLimit({ count: n });
						}
					}}
				/>
				<span class="dim">
					<PillSelect
						value={draft.limit.unit}
						options={[
							['songs', 'songs'],
							['minutes', 'minutes'],
							['gb', 'GB']
						]}
						label="Of what"
						size="small"
						strong
						onchange={(unit) => {
							draft.setLimit({ unit });
						}}
					/>
				</span>
				<span class="words">chosen by</span>
				<span class="dim">
					<PillSelect
						value={draft.limit.by}
						options={choosers}
						label="Chosen by"
						size="small"
						strong
						onchange={(by) => {
							draft.setChooser(by);
						}}
					/>
				</span>
			</div>
			<div class="order-row">
				<span class="gap"></span>
				<span>In the order of</span>
				<PillSelect
					value={draft.order}
					options={orderOptions}
					label="Order"
					size="small"
					strong
					onchange={(order: SmartOrder) => {
						draft.order = order;
					}}
				/>
			</div>
		</div>
	{/if}
{/snippet}

<div class="editor" class:phone bind:this={editor}>
	{#if phone}
		<div class="phone-head">
			<span class="kicker">EDITING THE RULES · SMART PLAYLIST</span>
			<input class="name" bind:value={draft.name} aria-label="Playlist name" maxlength="200" />
			<div class="phone-count" aria-live="polite">
				<span class="count" data-testid="smart-count" data-count={answer?.songCount}
					>{answer?.songCount.toLocaleString('en-US') ?? '…'}</span
				>
				<span class="word">{matchWord(answer?.songCount ?? 0)}</span>
				<span class="grow"></span>
				<a
					class="see"
					href="#songs"
					onclick={(event) => {
						event.preventDefault();
						document.getElementById('songs')?.scrollIntoView({ behavior: 'smooth' });
					}}>See them ›</a
				>
			</div>
		</div>
		<section class="phone-rules" aria-label="Rules">
			{@render ruleList()}
		</section>
		<SmartLive
			{answer}
			saved={changed ? savedCount : undefined}
			savedIds={changed ? savedIds : null}
			rows={40}
			phone
		/>
		<div class="phone-dock">
			<span class="hint">BACK LEAVES IT</span>
			<button type="button" class="leave" onclick={onleave}>Leave it</button>
			<button type="button" class="save" disabled={saving} onclick={onsave}>Save</button>
		</div>
	{:else}
		<div class="head">
			<span class="mosaic"><Mosaic albums={shownMosaic} size={72} radius="var(--radius-16)" /></span
			>
			<div class="titles">
				<span class="kicker">EDITING THE RULES · SMART PLAYLIST</span>
				<input class="name" bind:value={draft.name} aria-label="Playlist name" maxlength="200" />
			</div>
		</div>
		<div class="cards">
			<section class="rules" aria-label="Rules">
				<div class="scroll">
					{@render ruleList()}
				</div>
				<div class="dock">
					<span class="dock-words">
						<span class="dock-line">{dockLine(ruleCount(draft.rules), changed)}</span>
						<span class="hint">BACK OR ESC LEAVES IT</span>
					</span>
					<span class="grow"></span>
					<button type="button" class="leave" onclick={onleave}>Leave it</button>
					<button type="button" class="save" disabled={saving} onclick={onsave}>Save rules</button>
				</div>
			</section>
			<div class="side">
				<SmartLive
					{answer}
					saved={changed ? savedCount : undefined}
					savedIds={changed ? savedIds : null}
					rows={wide ? 15 : 13}
				/>
			</div>
		</div>
	{/if}
</div>

<style>
	.editor {
		display: flex;
		flex-direction: column;
		gap: calc(18px * var(--step));
		min-height: calc(100dvh - var(--frame-top) - 24px);
	}

	.head {
		display: flex;
		align-items: center;
		gap: calc(18px * var(--step));
	}

	.mosaic {
		flex: none;
		border-radius: var(--radius-16);
		box-shadow: 0 0 0 1px var(--edge);
	}

	.titles {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.kicker,
	.hint {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.name {
		width: 100%;
		box-sizing: border-box;
		padding: 0;
		border: 0;
		background: transparent;
		outline: none;
		color: var(--text);
		font: 900 calc(48px * var(--step)) / 1.05 var(--font-sans);
		letter-spacing: -0.05em;
	}

	.cards {
		flex: 1;
		min-height: 0;
		display: flex;
		align-items: stretch;
		gap: calc(18px * var(--step));
	}

	.rules {
		flex: 1 1 0;
		min-width: 0;
		display: flex;
		flex-direction: column;
		border-radius: var(--radius-24);
		background: var(--card);
		box-shadow: 0 0 0 1px var(--edge);
	}

	.scroll {
		flex: 1;
		padding: calc(22px * var(--step)) calc(24px * var(--step)) 8px;
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	.side {
		flex: 0 0 calc(440px * var(--step));
		min-width: 0;
		display: flex;
	}

	.side > :global(*) {
		flex: 1;
	}

	@media (min-width: 2560px) {
		.side {
			flex-basis: calc(620px * var(--step));
		}
	}

	.match-line {
		display: flex;
		align-items: center;
		gap: 10px;
		margin-bottom: 8px;
		font-size: 17px;
		font-weight: 700;
	}

	.group-row {
		display: flex;
		align-items: flex-start;
		gap: 8px;
		margin: 4px 0;
	}

	.join {
		flex: none;
		width: calc(58px * var(--step));
		padding-top: 18px;
		font-family: var(--font-mono);
		font-size: 11px;
		font-weight: 700;
		letter-spacing: 0.12em;
		color: var(--text-meta);
		transform: rotate(-2deg);
	}

	.join.fox {
		color: var(--fox-text);
	}

	.group {
		flex: 1;
		min-width: 0;
		padding: 10px 6px 8px 16px;
		border-left: 2px dashed color-mix(in srgb, var(--fox-line) 55%, transparent);
		border-radius: 0 var(--radius-16) var(--radius-16) 0;
		background: color-mix(in srgb, var(--fox) 4%, transparent);
	}

	.group-head {
		display: flex;
		align-items: center;
		gap: 8px;
		height: 44px;
		font-size: 14px;
		font-weight: 700;
	}

	.grow {
		flex: 1;
	}

	.quiet {
		height: 34px;
		padding: 0 12px;
		border: 0;
		border-radius: var(--radius-pill);
		background: none;
		color: var(--text-meta);
		font: 600 13px var(--font-sans);
	}

	.quiet:hover {
		background: var(--hover);
		color: var(--text);
	}

	.in-group-add,
	.add {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		border-radius: var(--radius-pill);
		background: none;
		font-family: var(--font-sans);
	}

	.in-group-add {
		height: 36px;
		margin-left: calc(42px * var(--step));
		padding: 0 12px;
		border: 0;
		color: var(--fox-text);
		font-size: 13px;
		font-weight: 700;
	}

	svg {
		width: 13px;
		height: 13px;
		fill: none;
		stroke: currentColor;
		stroke-width: 2.6;
		stroke-linecap: round;
	}

	.adds {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
		margin: 6px 0 0 calc(66px * var(--step));
	}

	.add {
		height: 40px;
		padding: 0 16px;
		gap: 8px;
		border: 1px dashed var(--edge-strong);
		color: var(--text-secondary);
		font-size: 14px;
		font-weight: 600;
	}

	.add.fox {
		border-color: color-mix(in srgb, var(--fox-line) 55%, transparent);
		color: var(--fox-text);
		font-weight: 700;
	}

	.add:hover {
		background: var(--hover);
	}

	.limits {
		margin-top: 18px;
		padding-top: 16px;
		border-top: 1px solid var(--edge);
		display: flex;
		flex-direction: column;
		gap: 10px;
	}

	.limit-row,
	.order-row {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 8px;
		min-height: 44px;
		font-size: 15px;
		font-weight: 600;
	}

	.limit-row.off .words {
		color: var(--text-meta);
	}

	.limit-row.off .dim,
	.limit-row.off .count {
		opacity: 0.45;
	}

	.switch {
		position: relative;
		width: 44px;
		height: 26px;
		margin-right: 6px;
		padding: 0;
		border: 0;
		border-radius: var(--radius-pill);
		background: var(--edge-strong);
		transition: background 160ms var(--curve-morph);
	}

	.switch[aria-checked='true'] {
		background: var(--fox);
	}

	.knob {
		position: absolute;
		top: 3px;
		left: 3px;
		width: 20px;
		height: 20px;
		border-radius: 50%;
		background: var(--text);
		transition: transform 180ms var(--curve-settle);
	}

	.switch[aria-checked='true'] .knob {
		transform: translateX(18px);
		background: var(--on-fox-ink);
	}

	.switch:focus-visible {
		outline: none;
		box-shadow: 0 0 0 3px var(--fox-line);
	}

	.count {
		width: 72px;
		height: 38px;
		box-sizing: border-box;
		padding: 0 10px;
		border: 0;
		border-bottom: 2px solid var(--fox-line);
		border-radius: 10px 10px 0 0;
		background: var(--hover);
		outline: none;
		color: var(--text);
		font: 600 15px var(--font-mono);
	}

	.dim {
		display: inline-flex;
	}

	.gap {
		width: 50px;
	}

	.dock {
		flex: none;
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 14px 18px 14px calc(24px * var(--step));
		border-top: 1px solid var(--edge);
	}

	.dock-words {
		display: flex;
		flex-direction: column;
		gap: 3px;
	}

	.dock-line {
		font-size: 14px;
		font-weight: 700;
	}

	.hint {
		font-size: 10px;
		letter-spacing: 0.1em;
	}

	.leave,
	.save {
		height: var(--target);
		border: 0;
		border-radius: var(--radius-pill);
		font: 600 15px var(--font-sans);
	}

	.leave {
		padding: 0 16px;
		background: none;
		color: var(--text-secondary);
	}

	.leave:hover {
		background: var(--hover);
	}

	.save {
		padding: 0 20px;
		background: var(--fox);
		color: var(--fox-ink);
		font-weight: 700;
	}

	.save:disabled {
		opacity: 0.6;
	}

	/* The phone boards. */
	.phone {
		gap: 0;
		min-height: 0;
	}

	.phone-head {
		position: sticky;
		top: 0;
		z-index: 2;
		margin: 0 calc(-1 * var(--gutter));
		padding: 12px var(--gutter) 12px;
		background: color-mix(in srgb, var(--ground) 94%, transparent);
		border-bottom: 1px solid var(--edge);
	}

	.phone-head .kicker {
		font-size: 10px;
	}

	.phone-head .name {
		margin: 4px 0 8px;
		font-size: 30px;
	}

	.phone-count {
		display: flex;
		align-items: baseline;
		gap: 8px;
	}

	.phone-count .count {
		width: auto;
		height: auto;
		padding: 0;
		border: 0;
		background: none;
		font-size: 32px;
		font-weight: 700;
		line-height: 1;
		color: var(--fox-text);
		text-shadow: 2px 1px 0 color-mix(in srgb, var(--misprint) 55%, transparent);
	}

	.phone-count .word {
		font-family: var(--font-mono);
		font-size: 11px;
		font-weight: 700;
		letter-spacing: 0.1em;
	}

	.see {
		font-size: 13px;
		font-weight: 700;
		color: var(--fox-text);
		text-decoration: none;
	}

	.phone-rules {
		display: flex;
		flex-direction: column;
		gap: 10px;
		padding: 14px 0;
	}

	.phone .match-line {
		font-size: 15px;
		margin: 0;
	}

	.phone .group-row {
		gap: 0;
	}

	.phone .join {
		width: 40px;
		padding: 14px 0 0 12px;
		font-size: 10px;
		letter-spacing: 0.1em;
		transform: none;
	}

	.phone .group {
		padding: 8px 6px 8px 12px;
	}

	.phone .adds {
		margin-left: 40px;
	}

	.phone .add.pill {
		border: 1px solid var(--edge-raised);
		border-style: solid;
		color: var(--text-meta);
		font-weight: 500;
	}

	.phone .limits {
		margin-top: 4px;
	}

	.phone .gap {
		display: none;
	}

	.phone-dock {
		position: sticky;
		bottom: 0;
		z-index: 2;
		margin: 0 calc(-1 * var(--gutter));
		padding: 12px var(--gutter) 24px;
		display: flex;
		align-items: center;
		gap: 8px;
		background: var(--card);
		border-top: 1px solid var(--edge);
	}

	.phone-dock .hint {
		flex: 1;
	}

	.phone-dock .leave {
		padding: 0 14px;
	}

	@media (prefers-reduced-motion: reduce) {
		.switch,
		.knob {
			transition: none;
		}
	}
</style>
