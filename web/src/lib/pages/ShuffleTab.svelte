<script lang="ts">
	import { tick } from 'svelte';
	import { clock } from '#lib/library/format.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import {
		combineOn,
		combinedElsewhere,
		excludedNote,
		orderOf
	} from '#lib/library/shuffleRules.ts';
	import Switch from './Switch.svelte';
	import type { EditSession } from './tagEditor.svelte.ts';

	interface Props {
		session: EditSession;
	}

	let { session }: Props = $props();

	const songs = $derived(session.tickedIds);
	const count = $derived(songs.length);
	const combined = $derived(combineOn(songs, session.rules, session.rulesDraft));
	const order = $derived(orderOf(songs, session.rules, session.rulesDraft));
	const elsewhere = $derived(combinedElsewhere(songs, session.rules));
	const excluded = $derived(session.allExcluded);
	const byId = $derived(new Map(session.tracks.map((track) => [track.id, track])));

	/** Where a dragged song started, while it's held. */
	let dragging = $state<number | null>(null);
	let list: HTMLOListElement | undefined = $state();
	let said = $state('');

	function titleOf(id: string): string {
		const track = byId.get(id);
		return track?.tags.fields.title?.at(0) ?? track?.tags.file ?? id;
	}

	function fromOf(id: string): string {
		const track = byId.get(id);
		if (track === undefined) {
			return '';
		}
		const album = library.album(track.albumId) ?? library.entry(track.albumId);
		const length = library.album(track.albumId)?.tracks.find((each) => each.id === id)?.durationMs;
		return [album?.title, length === undefined ? undefined : clock(length / 1000)]
			.filter((part) => part !== undefined)
			.join(' · ');
	}

	async function move(from: number, to: number): Promise<void> {
		if (to < 0 || to >= order.length) {
			return;
		}
		const id = order[from];
		session.moveInRun(from, to);
		said = `${titleOf(id ?? '')}, ${String(to + 1)} of ${String(order.length)}`;
		await tick();
		list?.querySelectorAll<HTMLButtonElement>('.handle')[to]?.focus();
	}

	function key(event: KeyboardEvent, index: number): void {
		if (!event.altKey || (event.key !== 'ArrowUp' && event.key !== 'ArrowDown')) {
			return;
		}
		event.preventDefault();
		void move(index, index + (event.key === 'ArrowUp' ? -1 : 1));
	}
</script>

<div class="shuffle">
	<Switch
		label="Combine"
		on={combined}
		disabled={count < 2}
		note={count < 2
			? 'Combine takes two or more songs.'
			: 'Always played together, in this order. Whichever one shuffle picks, they start from the first.'}
		toggle={(on) => {
			session.setCombine(on);
		}}
	/>
	{#if combined}
		<ol class="run" aria-label="The run's order" bind:this={list}>
			{#each order as id, index (id)}
				<li
					class:held={dragging === index}
					draggable="true"
					ondragstart={(event) => {
						dragging = index;
						event.dataTransfer?.setData('text/plain', id);
					}}
					ondragover={(event) => {
						event.preventDefault();
					}}
					ondrop={(event) => {
						event.preventDefault();
						if (dragging !== null) {
							void move(dragging, index);
						}
						dragging = null;
					}}
					ondragend={() => {
						dragging = null;
					}}
				>
					<span class="n">{index + 1}</span>
					<span class="what">
						<span class="title">{titleOf(id)}</span>
						<span class="from">{fromOf(id)}</span>
					</span>
					<button
						type="button"
						class="handle"
						aria-label={`Move ${titleOf(id)}. Alt with an arrow moves it.`}
						onkeydown={(event) => {
							key(event, index);
						}}
						><svg width="16" height="16" viewBox="0 0 24 24" aria-hidden="true">
							{#each [6, 12, 18] as y (y)}
								<circle cx="9" cy={y} r="1.6"></circle><circle cx="15" cy={y} r="1.6"></circle>
							{/each}
						</svg></button
					>
				</li>
			{/each}
		</ol>
	{/if}
	{#if elsewhere > 0}
		<p class="note">
			{elsewhere === 1 ? '1 of them is' : `${String(elsewhere)} of them are`} combined with other songs
			now. Combining these takes {elsewhere === 1 ? 'it' : 'them'} out of that run.
		</p>
	{/if}
	<Switch
		label={count === 1 ? 'Exclude from shuffle' : `Exclude all ${String(count)} from shuffle`}
		on={excluded}
		disabled={count === 0}
		note={`${excludedNote(songs, session.rules)} ${count === 1 ? 'Shuffle never picks it. It still plays when you choose it or play the album in order.' : 'Combined songs still play together when their run comes up.'}`}
		toggle={(on) => {
			session.setExclude(on);
		}}
	/>
	<p class="visually-hidden" role="status">{said}</p>
</div>

<style>
	.shuffle {
		display: flex;
		flex-direction: column;
		gap: 18px;
		margin-top: 18px;
	}

	.run {
		display: flex;
		flex-direction: column;
		gap: 6px;
		margin: 0 0 0 54px;
		padding: 0;
		list-style: none;
	}

	.run li {
		display: flex;
		align-items: center;
		gap: 14px;
		height: 56px;
		padding: 0 8px 0 14px;
		border-radius: var(--radius-16);
		background: var(--fill);
		cursor: grab;
	}

	.run li.held {
		opacity: 0.5;
	}

	.n {
		font-family: var(--font-mono);
		font-size: 13px;
		color: var(--fox-text);
	}

	.what {
		flex: 1;
		min-width: 0;
	}

	.title,
	.from {
		display: block;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.title {
		font-size: var(--type-body);
		font-weight: 600;
	}

	.from {
		font-size: 12px;
		color: var(--text-meta);
	}

	.handle {
		flex: none;
		width: 40px;
		height: 40px;
		display: grid;
		place-items: center;
		border: 0;
		border-radius: var(--radius-pill);
		background: transparent;
		cursor: grab;
	}

	.handle circle {
		fill: var(--text-label);
	}

	.handle:focus-visible circle,
	.handle:hover circle {
		fill: var(--text);
	}

	.note {
		margin: 0;
		font-size: 13px;
		line-height: 1.5;
		color: var(--text-meta);
	}

	@media (max-width: 480px) {
		.run {
			margin-left: 0;
		}
	}
</style>
