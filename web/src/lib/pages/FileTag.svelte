<!--
	The file mark on a file playlist's mosaic: its top right corner dog-eared down, a paper triangle
	with a small shadow, and a paper tag on the lower left with the file glyph and the extension as
	written, in a dashed ink border, turned +2°: the other way from SMART's −3°, and with no misprint,
	so the two never read as one.
-->
<script lang="ts">
	import { tagWords } from './fileWords.ts';

	interface Props {
		format: 'm3u' | 'm3u8';
		/** Card, the page's 200px mosaic, or a phone's 112px one. */
		size?: 'card' | 'page' | 'phone';
	}

	let { format, size = 'card' }: Props = $props();
</script>

<span class={['ear', size]} aria-hidden="true"></span>
<span class={['tag', size]}
	>{#if size === 'card'}<svg
			width="11"
			height="13"
			viewBox="0 0 11 13"
			fill="none"
			stroke="currentColor"
			stroke-width="1.6"
			stroke-linejoin="round"
			aria-hidden="true"><path d="M1 1h6l3 3v8H1z"></path><path d="M7 1v3h3"></path></svg
		>{/if}<span class="words">{tagWords(format)}</span><span class="visually-hidden">
		file playlist</span
	></span
>

<style>
	/* The fold: the page's ground over the corner, the paper's back under it. */
	.ear {
		position: absolute;
		top: 0;
		right: 0;
		width: calc(38px * var(--step));
		height: calc(38px * var(--step));
		border-radius: 0 var(--radius-24) 0 10px;
		background: linear-gradient(225deg, var(--ground) 50%, var(--paper) 50%);
		box-shadow: -2px 2px 4px color-mix(in srgb, var(--paper-ink) 40%, transparent);
		pointer-events: none;
	}

	.ear.page {
		width: calc(34px * var(--step));
		height: calc(34px * var(--step));
		border-radius: 0 var(--radius-24) 0 9px;
	}

	.ear.phone {
		width: 24px;
		height: 24px;
		border-radius: 0 var(--radius-16) 0 7px;
	}

	.tag {
		position: absolute;
		left: calc(12px * var(--step));
		bottom: calc(12px * var(--step));
		display: flex;
		align-items: center;
		gap: 6px;
		height: calc(28px * var(--step));
		padding: 0 calc(10px * var(--step));
		box-sizing: border-box;
		border-radius: 4px;
		border: 1.5px dashed var(--paper-line);
		background: var(--paper);
		color: var(--paper-ink);
		font-family: var(--font-mono);
		font-size: calc(11px * var(--step));
		font-weight: 700;
		letter-spacing: 0.12em;
		transform: rotate(2deg);
		pointer-events: none;
	}

	.tag.page {
		gap: 5px;
		height: calc(26px * var(--step));
		padding: 0 calc(9px * var(--step));
		font-size: calc(10px * var(--step));
	}

	.tag.phone {
		left: 7px;
		bottom: 7px;
		height: 20px;
		padding: 0 6px;
		border-radius: 3px;
		font-size: 9px;
		letter-spacing: 0.1em;
	}

	.visually-hidden {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip-path: inset(50%);
		white-space: nowrap;
	}
</style>
