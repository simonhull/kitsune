<script lang="ts">
	import { onMount } from 'svelte';
	import { formatOf, gain, gainMeasured, trackGain } from '#lib/library/format.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import type { TrackTags } from '#lib/library/source.ts';
	import { tagGroups } from '#lib/library/tags.ts';

	interface Props {
		albumId: string;
		trackId: string;
	}

	let { albumId, trackId }: Props = $props();

	const album = $derived(library.album(albumId));
	const track = $derived(album?.tracks.find(({ id }) => id === trackId));
	/** The file's own tags, as the tag editor reads them: only for those who may read them. */
	let tags = $state<TrackTags | null>(null);
	let reading = $state(true);

	const labels = new Map(
		tagGroups.flatMap(({ fields }) =>
			fields.map(({ field, label, name }) => [field, name ?? label])
		)
	);

	const facts = $derived.by(() => {
		if (track === undefined) {
			return [];
		}
		const rg = trackGain(track);
		return [
			['Format', formatOf(track)],
			...(rg === null
				? []
				: [['Track gain', `${gain(rg)} dB${gainMeasured(track) ? ', measured by Kitsune' : ''}`]]),
			...(track.year === undefined ? [] : [['Year', String(track.year)]]),
			...(track.genres.length === 0 ? [] : [['Genre', track.genres.join(', ')]])
		];
	});

	onMount(() => {
		void library.loadAlbum(albumId);
		if (!library.admin) {
			reading = false;
			return;
		}
		library
			.albumTags(albumId)
			.then((all) => {
				tags = all.tracks.find(({ id }) => id === trackId) ?? null;
			})
			.catch(() => undefined)
			.finally(() => {
				reading = false;
			});
	});
</script>

<div class="info" data-menu-item tabindex="-1">
	<p class="eyebrow">FILE INFO</p>
	{#if tags !== null}
		<p class="file" title={tags.file}>{tags.file}</p>
	{/if}
	<dl>
		{#each facts as [name, value] (name)}
			<dt>{name}</dt>
			<dd>{value}</dd>
		{/each}
		{#if tags !== null}
			{#each Object.entries(tags.fields) as [field, values] (field)}
				{#if values.length > 0 && field !== 'lyrics'}
					<dt>{labels.get(field as never) ?? field}</dt>
					<dd>{values.join(', ')}</dd>
				{/if}
			{/each}
			{#each tags.tags as raw (raw.key)}
				<dt class="raw">{raw.key}</dt>
				<dd>{raw.values.join(', ')}</dd>
			{/each}
		{/if}
	</dl>
	{#if reading}
		<p class="note" role="status">Reading the file…</p>
	{:else if tags === null && library.admin}
		<p class="note" role="status">The file's tags couldn't be read.</p>
	{/if}
</div>

<style>
	.info {
		max-height: min(60dvh, 560px);
		overflow-y: auto;
		padding: 4px 12px 8px;
		outline: none;
	}

	.eyebrow {
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.file {
		margin-top: 6px;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		overflow-wrap: anywhere;
		color: var(--text-secondary);
	}

	dl {
		margin-top: 10px;
		display: grid;
		grid-template-columns: auto 1fr;
		gap: 6px 12px;
		font-size: 13px;
	}

	dt {
		color: var(--text-meta);
	}

	dt.raw {
		font-family: var(--font-mono);
		font-size: 11px;
	}

	dd {
		min-width: 0;
		overflow-wrap: anywhere;
		color: var(--text);
	}

	.note {
		margin-top: 10px;
		font-size: 13px;
		color: var(--text-meta);
	}
</style>
