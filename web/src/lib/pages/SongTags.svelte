<!--
	One song's every tag, in the tag editor's right column: its fields by group, its lyrics (read-only
	when timestamped), every other tag it holds, and its own Exclude switch.
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import {
		isChanged,
		isTagChanged,
		tagValueOf,
		valueOf,
		wasOf,
		type EditTrack
	} from '#lib/library/edit.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import { fieldId, tagGroups, tagId } from '#lib/library/tags.ts';
	import ChipField from './ChipField.svelte';
	import Switch from './Switch.svelte';
	import TagField from './TagField.svelte';
	import type { EditSession } from './tagEditor.svelte.ts';

	interface Props {
		track: EditTrack;
		session: EditSession;
		/** Goes back to every ticked song at once. */
		close: () => void;
	}

	let { track, session, close }: Props = $props();

	const details = $derived(library.album(track.albumId)?.tracks.find(({ id }) => id === track.id));
	const lyricsId = $derived(fieldId(track.id, 'lyrics'));
	const lyricsFixed = $derived(track.tags.readOnly?.includes('lyrics') ?? false);
	const lyricsChanged = $derived(isChanged(track, session.draft, session.ticked, 'lyrics'));
	const title = $derived(
		valueOf(track, session.draft, session.ticked, 'title').at(0) ?? track.tags.file
	);
	const quality = $derived(
		[
			details?.format,
			details?.bitDepth === undefined || details.sampleRate === undefined
				? undefined
				: `${String(details.bitDepth)}/${String(details.sampleRate / 1000)}`
		]
			.filter((part) => part !== undefined)
			.join(' ')
	);

	let panel: HTMLElement | undefined = $state();

	onMount(() => {
		panel?.focus({ preventScroll: true });
	});
</script>

<div class="song" role="group" aria-label={`Every tag of ${title}`} tabindex="-1" bind:this={panel}>
	<div class="top">
		<button type="button" class="back" onclick={close}
			><svg width="16" height="16" viewBox="0 0 24 24" aria-hidden="true"
				><path d="M15 6l-6 6 6 6"></path></svg
			>All {session.tracks.length} songs</button
		>
		<span class="file"
			>{track.tags.file}{#if quality !== ''}&nbsp;· {quality}{/if}</span
		>
	</div>
	<h2>{title}</h2>
	{#each tagGroups as group (group.name)}
		<div class="group">
			<h3>{group.name}</h3>
			<div class="grid">
				{#each group.fields as spec (spec.field)}
					<TagField {spec} {track} {session} />
				{/each}
			</div>
		</div>
	{/each}
	<div class="group">
		<label class="heading" for={lyricsId}>
			LYRICS{#if lyricsFixed}&nbsp;· TIMESTAMPED, READ-ONLY{/if}
		</label>
		<textarea
			id={lyricsId}
			class:changed={lyricsChanged}
			readonly={lyricsFixed}
			placeholder="Empty"
			bind:value={
				() => valueOf(track, session.draft, session.ticked, 'lyrics').at(0) ?? '',
				(value) => {
					session.setField(track.id, 'lyrics', [value]);
				}
			}></textarea>
		{#if lyricsChanged && wasOf(track, 'lyrics').length > 0}
			<span class="note">The file's lyrics go when this is written.</span>
		{/if}
	</div>
	<div class="group">
		<h3 class="heading">EVERY OTHER TAG · {track.tags.tags.length}</h3>
		<ul aria-label="Every other tag">
			{#each track.tags.tags as { key, values, readOnly = false }, place (place)}
				{@const id = tagId(track.id, key)}
				{@const shown = tagValueOf(track, session.draft, key)}
				{@const changed = isTagChanged(track, session.draft, key)}
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
	<div class="group rules">
		<h3>IN YOUR SHUFFLE · NOT A TAG, NEVER WRITTEN TO THE FILE</h3>
		<Switch
			label="Exclude from shuffle"
			on={session.isExcluded(track.id)}
			note="Shuffle never picks it. It still plays when you choose it or play the album in order."
			toggle={(on) => {
				session.setExcluded(track.id, on);
			}}
		/>
	</div>
</div>

<style>
	.song {
		display: flex;
		flex-direction: column;
		gap: 14px;
		outline: none;
		animation: arrive var(--morph-fast) var(--curve-morph) both;
	}

	@keyframes arrive {
		from {
			opacity: 0;
			translate: 0 8px;
		}
	}

	.top {
		display: flex;
		align-items: center;
		gap: 10px;
		min-width: 0;
	}

	.back {
		flex: none;
		display: flex;
		align-items: center;
		gap: 4px;
		height: 40px;
		padding: 0 14px 0 8px;
		border: 0;
		border-radius: var(--radius-pill);
		background: var(--input);
		font-size: 14px;
		font-weight: 600;
	}

	.back svg,
	.back path {
		fill: none;
		stroke: currentColor;
		stroke-width: 2.2;
		stroke-linecap: round;
		stroke-linejoin: round;
	}

	.file {
		min-width: 0;
		overflow: hidden;
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--text-meta);
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	h2 {
		font-size: 28px;
		font-weight: 800;
		letter-spacing: -0.03em;
		overflow-wrap: anywhere;
	}

	.group {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	h3,
	.heading {
		margin: 0;
		font-family: var(--font-mono);
		font-size: 11px;
		font-weight: 400;
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.grid {
		display: grid;
		grid-template-columns: repeat(4, minmax(0, 1fr));
		gap: 8px 20px;
	}

	textarea {
		display: block;
		width: 100%;
		height: 70px;
		padding: 10px 12px;
		border-radius: 14px;
		border: 1px solid var(--edge-field);
		outline: none;
		background: transparent;
		color: var(--text);
		font: inherit;
		font-size: 14px;
		line-height: 1.5;
		resize: vertical;
	}

	textarea:focus {
		border-color: var(--text);
	}

	textarea::placeholder {
		font-style: italic;
		color: var(--text-meta);
	}

	textarea[readonly] {
		color: var(--text-meta);
		border-color: var(--edge-card);
	}

	textarea.changed {
		border-color: var(--fox-line);
	}

	.note {
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--text-meta);
	}

	ul {
		margin: 0;
		padding: 0;
		list-style: none;
	}

	li {
		display: grid;
		grid-template-columns: minmax(0, 210px) minmax(0, 1fr) auto;
		align-items: center;
		column-gap: 12px;
		min-height: 32px;
		border-bottom: 1px solid var(--edge-card);
		font-size: 13px;
	}

	li.changed {
		border-bottom-color: var(--fox-line);
	}

	li:focus-within {
		border-bottom-color: var(--text);
	}

	.key {
		overflow: hidden;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		color: var(--text-meta);
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.value,
	li input {
		min-width: 0;
		overflow: hidden;
		font-weight: 600;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	li input {
		width: 100%;
		height: 30px;
		padding: 0;
		border: 0;
		outline: none;
		background: transparent;
		color: var(--text);
		font: inherit;
		font-weight: 600;
	}

	li input::placeholder {
		font-style: italic;
		font-weight: 400;
		color: var(--text-meta);
	}

	.fixed .value {
		font-weight: 400;
		color: var(--text-label);
	}

	.was {
		max-width: 12ch;
		overflow: hidden;
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--text-meta);
		text-decoration: line-through;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.rules {
		gap: 12px;
		padding-top: 14px;
		border-top: 1px solid var(--edge-card);
	}

	/* Inside a narrow column, a phone's, the groups go two to a row. */
	@container (max-width: 480px) {
		.grid {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}

		li {
			grid-template-columns: minmax(0, 40%) minmax(0, 1fr) auto;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.song {
			animation: none;
		}
	}
</style>
