<script lang="ts">
	import { count } from '#lib/library/home.ts';
	import { minutesLeft, steadyMinutes } from '#lib/library/health.ts';
	import type { FingerprintStatus, ScanStatus } from '#lib/library/source.ts';

	/**
	 * A scan reading files, or, quieter, the fingerprint pass behind it. A slow scan never looks
	 * stalled: the count moves as files are read.
	 */
	type Props =
		{ kind: 'scan'; scan: ScanStatus; now: number } | { kind: 'pass'; pass: FingerprintStatus };

	let props: Props = $props();

	/** The estimate on screen, held until the real one has clearly moved. */
	let shown: number | null = null;

	const done = $derived(props.kind === 'scan' ? props.scan.read : props.pass.done);
	const total = $derived(props.kind === 'scan' ? props.scan.total : props.pass.total);
	const percent = $derived(total === 0 ? 0 : Math.min(100, (done / total) * 100));
	const left = $derived.by(() => {
		if (props.kind !== 'scan') {
			return null;
		}
		shown = steadyMinutes(shown, minutesLeft(props.scan, props.now));
		if (shown === null) {
			return null;
		}
		return shown < 1 ? 'under a minute left' : `${count(shown)} min left`;
	});
	const figures = $derived.by(() => {
		if (total === 0) {
			return 'looking for files';
		}
		const of =
			props.kind === 'scan'
				? `${count(done)} of about ${count(total)}`
				: `${count(done)} of ${count(total)}`;
		return left === null ? of : `${of} · ${left}`;
	});
</script>

<div class="scan-card" class:pass={props.kind === 'pass'} role="status">
	<div class="head">
		<span class="title">
			{props.kind === 'scan' ? 'Reading your library' : 'Fingerprinting, in the background'}
		</span>
		<span class="figures">{figures}</span>
	</div>
	<div class="bar" aria-hidden="true">
		<div class="fill" style:width={`${String(percent)}%`}></div>
	</div>
	<span class="note">
		{props.kind === 'scan'
			? 'Songs appear as they’re read. You can play anything already here.'
			: 'Lets Kitsune follow a song you move or rename. Nothing waits on it.'}
	</span>
</div>

<style>
	.scan-card {
		box-sizing: border-box;
		padding: var(--scan-pad, 18px 22px);
		border-radius: var(--radius-20);
		background: var(--card);
		border: 1px solid var(--edge);
		display: flex;
		flex-direction: column;
		gap: 12px;
		animation: arrive var(--morph) var(--curve-morph) both;
	}

	.head {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		justify-content: space-between;
		gap: 4px 16px;
	}

	.title {
		font-size: var(--type-body);
		font-weight: 700;
	}

	.pass .title {
		color: var(--text-secondary);
	}

	.figures {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		font-variant-numeric: tabular-nums;
		color: var(--text-meta);
	}

	.bar {
		height: 6px;
		border-radius: var(--radius-pill);
		background: var(--fill);
		overflow: hidden;
	}

	.fill {
		height: 100%;
		border-radius: var(--radius-pill);
		background: var(--fox);
		transition: width var(--morph) var(--curve-morph);
	}

	.pass .fill {
		background: color-mix(in srgb, var(--fox) 45%, transparent);
	}

	.note {
		font-size: 13px;
		line-height: 1.5;
		color: var(--text-meta);
	}

	@keyframes arrive {
		from {
			opacity: 0;
			transform: translateY(8px);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.scan-card {
			animation-name: none;
		}

		.fill {
			transition: none;
		}
	}
</style>
