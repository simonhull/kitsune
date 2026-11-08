<script lang="ts">
	import { onMount } from 'svelte';
	import Cover from '#lib/cover/Cover.svelte';
	import type { Album, AlbumTags, TrackTags } from '#lib/library/source.ts';
	import {
		fieldId,
		isChanged,
		isTagChanged,
		tagGroups,
		tagId,
		tagValueOf,
		valueOf,
		wasOf
	} from '#lib/library/tags.ts';
	import { reducedMotion } from '#lib/morph/journey.ts';
	import ChipField from './ChipField.svelte';
	import TagField from './TagField.svelte';
	import type { TagSession } from './tagEdits.svelte.ts';

	interface Props {
		album: Album;
		tags: AlbumTags;
		track: TrackTags;
		session: TagSession;
		/** The cover is coming down from the stage. */
		fly: boolean;
		/** Goes back to every track at once. */
		close: () => void;
	}

	let { album, tags, track, session, fly, close }: Props = $props();

	const details = $derived(album.tracks.find(({ id }) => id === track.id));
	const lyricsId = $derived(fieldId(track.id, 'lyrics'));
	const lyricsFixed = $derived(track.readOnly?.includes('lyrics') ?? false);
	const lyricsChanged = $derived(isChanged(tags, session.draft, track.id, 'lyrics'));
	const quality = $derived(
		[
			details?.format,
			details?.bitDepth === undefined || details.sampleRate === undefined
				? undefined
				: `${String(details.bitDepth)}/${String(details.sampleRate / 1000)}`
		]
			.filter((part) => part !== undefined)
			.join(' · ')
	);
	const artwork = $derived(album.artwork);

	let panel: HTMLElement | undefined = $state();
	let art: HTMLElement | undefined = $state();

	/** The cover leaves the stage for the panel: it flies from where it stood. */
	function flyCover(): void {
		const from = document.querySelector('[data-morph-part="art"]')?.getBoundingClientRect();
		const to = art?.getBoundingClientRect();
		if (from === undefined || to === undefined || from.width === 0 || reducedMotion()) {
			return;
		}
		const x = from.left - to.left;
		const y = from.top - to.top;
		art?.animate(
			[
				{
					transformOrigin: 'top left',
					transform: `translate(${String(x)}px, ${String(y)}px) scale(${String(from.width / to.width)})`
				},
				{ transformOrigin: 'top left', transform: 'none' }
			],
			{ duration: 420, easing: 'cubic-bezier(0.22, 1, 0.36, 1)' }
		);
	}

	onMount(() => {
		if (fly) {
			flyCover();
		}
		panel?.focus({ preventScroll: true });
	});
</script>

<div
	class="open"
	role="group"
	aria-label={`Every tag of ${track.fields.title?.at(0) ?? track.file}`}
	tabindex="-1"
	bind:this={panel}
>
	<div class="top">
		<div class="side">
			<div class="art" bind:this={art}>
				<Cover albumId={album.id} entry={album} place="card" />
			</div>
			<button type="button" class="back" onclick={close}>All tracks</button>
			<p class="facts">
				{#if quality !== ''}{quality}<br />{/if}
				{#if artwork !== undefined}
					{artwork.width.toLocaleString('en-US')} × {artwork.height.toLocaleString('en-US')}<br />
				{/if}
				{track.file}
			</p>
		</div>
		<div class="groups">
			{#each tagGroups as group (group.name)}
				<div>
					<h3>{group.name}</h3>
					<div class="grid">
						{#each group.fields as spec (spec.field)}
							<TagField {spec} trackId={track.id} {tags} {session} />
						{/each}
					</div>
				</div>
			{/each}
		</div>
	</div>
	<div class="bottom">
		<div class="lyrics">
			<label class="heading" for={lyricsId}>
				LYRICS{#if lyricsFixed}&nbsp;· TIMESTAMPED, READ-ONLY{/if}
			</label>
			<textarea
				id={lyricsId}
				class:changed={lyricsChanged}
				readonly={lyricsFixed}
				placeholder="Empty"
				bind:value={
					() => valueOf(tags, session.draft, track.id, 'lyrics').at(0) ?? '',
					(value) => {
						session.setField(track.id, 'lyrics', [value]);
					}
				}></textarea>
			{#if lyricsChanged && wasOf(tags, track.id, 'lyrics').length > 0}
				<span class="note">The file's lyrics go when this is written.</span>
			{/if}
		</div>
		<div class="others">
			<h3 class="heading">EVERY OTHER TAG · {track.tags.length}</h3>
			<!-- It scrolls, and read-only rows hold no focus. -->
			<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
			<ul tabindex="0" aria-label="Every other tag">
				{#each track.tags as { key, values, readOnly = false }, place (place)}
					{@const id = tagId(track.id, key)}
					{@const shown = tagValueOf(tags, session.draft, track.id, key)}
					{@const changed = isTagChanged(tags, session.draft, track.id, key)}
					<li class:changed class:fixed={readOnly}>
						<span class="key" title={key}>{key}</span>
						{#if readOnly}
							<span class="value" title={values.join(', ')}>{values.join(', ')}</span>
						{:else if values.length > 1}
							<ChipField
								{id}
								label={key}
								placeholder="Empty"
								bind:values={
									() => shown,
									(next) => {
										session.setTag(track.id, key, next);
									}
								}
							/>
						{:else}
							<input
								{id}
								type="text"
								autocomplete="off"
								aria-label={key}
								placeholder="Removed when written"
								bind:value={
									() => shown.at(0) ?? '',
									(value) => {
										session.setTag(track.id, key, [value]);
									}
								}
							/>
							{#if changed}
								<span class="was"><span class="visually-hidden">was </span>{values.at(0)}</span>
							{/if}
						{/if}
					</li>
				{:else}
					<li class="fixed"><span class="value">None</span></li>
				{/each}
			</ul>
		</div>
	</div>
</div>

<style>
	.open {
		margin: 10px -8px;
		padding: 20px 24px 24px;
		border-radius: var(--radius-20);
		background: var(--card-raised);
		box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--fox) 50%, transparent);
		outline: none;
		scroll-margin: 96px 0 200px;
		animation: unfold var(--morph) var(--curve-morph) both;
	}

	@keyframes unfold {
		from {
			opacity: 0;
			translate: 0 -8px;
		}
	}

	.top,
	.bottom {
		display: flex;
		gap: 24px;
	}

	.bottom {
		margin-top: 20px;
	}

	.side {
		flex: none;
		width: 176px;
	}

	.art {
		position: relative;
		z-index: 1;
		width: 176px;
		height: 176px;
		border-radius: 18px;
		overflow: hidden;
	}

	.back {
		width: 100%;
		height: var(--target);
		margin-top: 10px;
		border-radius: var(--radius-pill);
		border: 1px solid var(--edge-strong);
		background: transparent;
		font-size: 14px;
		font-weight: 600;
	}

	.facts {
		margin-top: 10px;
		font-family: var(--font-mono);
		font-size: 11px;
		line-height: 1.6;
		color: var(--text-meta);
		overflow-wrap: anywhere;
	}

	.groups {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 18px;
	}

	h3 {
		margin: 0 0 6px;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		font-weight: 400;
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.grid {
		display: grid;
		grid-template-columns: repeat(4, minmax(0, 1fr));
		gap: 8px 20px;
	}

	.lyrics,
	.others {
		flex: 1;
		min-width: 0;
	}

	.heading {
		display: block;
		margin-bottom: 6px;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.lyrics textarea {
		display: block;
		width: 100%;
		height: 150px;
		padding: 12px;
		border-radius: 14px;
		border: 1px solid var(--edge-field);
		outline: none;
		background: var(--card);
		color: var(--text);
		font: inherit;
		font-size: 14px;
		line-height: 1.5;
		resize: none;
	}

	.lyrics textarea:focus {
		border-color: var(--text);
	}

	.lyrics textarea[readonly] {
		color: var(--text-meta);
		border-color: var(--edge-card);
	}

	.lyrics textarea.changed {
		border-color: var(--fox-text);
	}

	.lyrics .note {
		display: block;
		margin-top: 6px;
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--text-meta);
	}

	.others ul {
		max-height: 150px;
		margin: 0;
		padding: 0;
		overflow-y: auto;
		list-style: none;
		scrollbar-width: thin;
	}

	.others li {
		display: flex;
		align-items: center;
		gap: 12px;
		min-height: 34px;
		border-bottom: 1px solid var(--edge-card);
		font-family: var(--font-mono);
		font-size: var(--type-mono);
	}

	.others li.changed {
		border-bottom-color: var(--fox-text);
	}

	.others li:focus-within {
		border-bottom-color: var(--text);
	}

	.key {
		flex: none;
		width: 190px;
		overflow: hidden;
		color: var(--text-meta);
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.value,
	.others input {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.others input {
		height: 32px;
		padding: 0;
		border: 0;
		outline: none;
		background: transparent;
		color: var(--text);
		font: inherit;
	}

	.others input::placeholder {
		color: var(--text-label);
	}

	.fixed .value {
		color: var(--text-label);
	}

	.was {
		flex: 0 1 auto;
		max-width: 40%;
		overflow: hidden;
		font-size: 11px;
		color: var(--text-meta);
		text-decoration: line-through;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	@media (max-width: 1180px) {
		.grid {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}

		.bottom {
			flex-direction: column;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.open {
			animation: none;
		}
	}
</style>
