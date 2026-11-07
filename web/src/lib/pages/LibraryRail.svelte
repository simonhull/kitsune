<script lang="ts">
	interface Props {
		letters: readonly string[];
		/** Letters with nothing shown under them: grey, and a jump to one goes on to the next. */
		quiet: ReadonlySet<string>;
		/** The letter you're at. */
		here: string | undefined;
		onjump: (letter: string) => void;
	}

	let { letters, quiet, here, onjump }: Props = $props();

	let rail: HTMLElement | undefined = $state();

	/** A finger slides down the rail, jumping as it goes, so a touch here must not scroll the page. */
	function slide({ clientY }: PointerEvent): void {
		if (rail !== undefined) {
			const { top, height } = rail.getBoundingClientRect();
			const place = Math.floor(((clientY - top) / height) * letters.length);
			const letter = letters.at(Math.min(letters.length - 1, Math.max(0, place)));
			if (letter !== undefined) {
				onjump(letter);
			}
		}
	}

	function onpointerdown(event: PointerEvent): void {
		if (event.pointerType !== 'mouse') {
			rail?.setPointerCapture(event.pointerId);
			slide(event);
		}
	}

	function onpointermove(event: PointerEvent): void {
		if (rail?.hasPointerCapture(event.pointerId) === true) {
			slide(event);
		}
	}
</script>

<nav
	class="rail"
	aria-label="Jump to a letter"
	style:--letters={letters.length}
	bind:this={rail}
	{onpointerdown}
	{onpointermove}
>
	{#each letters as letter (letter)}
		<a
			href={`#${letter === '#' ? 'other' : letter}`}
			class:quiet={quiet.has(letter)}
			aria-current={letter === here ? 'true' : undefined}
			onclick={(event) => {
				event.preventDefault();
				onjump(letter);
			}}>{letter}</a
		>
	{/each}
</nav>

<style>
	.rail {
		--room: calc(100dvh - 216px);
		--step: min(25px, calc(var(--room) / var(--letters)));

		position: fixed;
		z-index: 1;
		top: 104px;
		right: 24px;
		width: 32px;
		display: flex;
		flex-direction: column;
		align-items: center;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		color: var(--text-meta);
	}

	a {
		width: 28px;
		height: var(--step);
		display: flex;
		align-items: center;
		justify-content: center;
		position: relative;
	}

	/* The board's pill, 22 tall in a 25 step: the step shrinks in a short window, and it shrinks too. */
	a::before {
		content: '';
		position: absolute;
		inset: 1.5px 0;
		border-radius: var(--radius-pill);
	}

	a:hover {
		color: var(--fox);
	}

	.quiet,
	.quiet:hover {
		color: color-mix(in srgb, var(--text-label) 45%, var(--ground));
	}

	[aria-current='true'] {
		color: var(--fox-ink);
		font-weight: 700;
	}

	[aria-current='true']:hover {
		color: var(--fox-ink);
	}

	[aria-current='true']::before {
		z-index: -1;
		background: var(--fox);
	}

	@media (max-width: 1024px) {
		.rail {
			--room: calc(100dvh - 320px);
			--step: min(44px, calc(var(--room) / var(--letters)));

			top: 216px;
			right: 2px;
			width: 36px;
			touch-action: none;
		}

		a::before {
			inset: 0;
		}
	}
</style>
