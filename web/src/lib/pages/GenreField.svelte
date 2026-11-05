<script lang="ts">
	import Icon from '#lib/shell/Icon.svelte';

	interface Props {
		id: string;
		genres: string[];
	}

	let { id, genres = $bindable() }: Props = $props();

	let typing = $state('');
	let input: HTMLInputElement | undefined = $state();

	/** Takes what's been typed. A genre from a file may hold a comma; a typed one ends at it. */
	function take(): void {
		for (const part of typing.split(',')) {
			const genre = part.trim().replace(/\s+/g, ' ');
			const folded = genre.toLowerCase();
			if (genre !== '' && !genres.some((other) => other.toLowerCase() === folded)) {
				genres = [...genres, genre];
			}
		}
		typing = '';
	}

	/** Enter takes a genre while there's one to take. With none it's left to save the form. */
	function key(event: KeyboardEvent): void {
		if (event.key === ',' || (event.key === 'Enter' && typing.trim() !== '')) {
			event.preventDefault();
			take();
		}
	}

	function remove(gone: string): void {
		genres = genres.filter((genre) => genre !== gone);
		input?.focus();
	}
</script>

<div class="genres">
	<label for={id}>Genres</label>
	<div class="field">
		{#if genres.length > 0}
			<ul>
				{#each genres as genre (genre)}
					<li>
						<button
							type="button"
							aria-label={`Remove ${genre}`}
							onclick={() => {
								remove(genre);
							}}
						>
							<span>{genre}</span>
							<Icon name="close" size={14} />
						</button>
					</li>
				{/each}
			</ul>
		{/if}
		<input
			{id}
			type="text"
			autocomplete="off"
			placeholder="Add a genre"
			bind:this={input}
			bind:value={typing}
			onkeydown={key}
			onblur={take}
		/>
	</div>
</div>

<style>
	.genres {
		display: flex;
		flex-direction: column;
		gap: 6px;
		min-width: 0;
	}

	label {
		font-size: 13px;
		font-weight: 600;
		color: var(--text-meta);
	}

	.field,
	ul {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
		min-width: 0;
	}

	ul {
		margin: 0;
		padding: 0;
		list-style: none;
	}

	li {
		min-width: 0;
	}

	button {
		display: flex;
		align-items: center;
		gap: 8px;
		max-width: 100%;
		min-height: var(--target);
		padding: 0 14px 0 16px;
		border: 1px solid var(--edge-strong);
		border-radius: var(--radius-pill);
		background: none;
		font-size: 14px;
		font-weight: 600;
		text-align: left;
	}

	button span {
		min-width: 0;
		overflow-wrap: anywhere;
	}

	input {
		flex: 1;
		min-width: 160px;
		height: var(--target);
		padding: 0 16px;
		border: 1px solid var(--edge-strong);
		border-radius: var(--radius-pill);
		background: rgb(10 9 8 / 30%);
		color: var(--text);
		font: inherit;
		font-size: 16px;
	}

	input::placeholder {
		color: var(--text-label);
	}
</style>
