<!--
	Server settings' Matching card, board E. MusicBrainz needs no key and is always on; matching by
	sound with AcoustID, and Discogs, come later, each with its own key.
-->
<section class="card" aria-labelledby="matching-settings">
	<h2 id="matching-settings">Matching</h2>
	<p class="lede">
		Find matches asks MusicBrainz for an album’s release, and needs no key. Matching songs by their
		sound, for files whose tags are wrong or empty, comes later.
	</p>
	<div class="source">
		<span class="words"
			><span class="name">MusicBrainz</span><span class="note"
				>No key needed · answers are kept for a day</span
			></span
		>
		<span class="mono">ALWAYS ON</span>
	</div>
	<div class="source later">
		<span class="words"
			><span class="name">AcoustID</span><span class="note"
				>Matches songs by their sound, with a free key</span
			></span
		>
		<span class="mono">LATER</span>
	</div>
	<div class="source later">
		<span class="words"
			><span class="name">Discogs</span><span class="note"
				>For labels, catalog numbers and pressings MusicBrainz is thin on</span
			></span
		>
		<span class="mono">LATER</span>
	</div>
</section>

<style>
	.card {
		box-sizing: border-box;
		padding: 32px;
		display: flex;
		flex-direction: column;
		gap: 20px;
		border-radius: var(--radius-28);
		background: var(--card);
		border: 1px solid var(--edge-card);
	}

	h2 {
		font-size: var(--type-section);
		font-weight: 800;
		letter-spacing: var(--tracking-section);
	}

	.lede {
		font-size: 14px;
		line-height: 1.55;
		color: var(--text-secondary);
	}

	.source {
		display: flex;
		align-items: center;
		gap: 14px;
		min-height: 60px;
		padding: 10px 18px;
		box-sizing: border-box;
		border-radius: 18px;
		background: var(--input);
	}

	.source.later {
		background: transparent;
		border: 1px dashed var(--edge-strong);
		color: var(--text-meta);
	}

	.words {
		flex: 1;
		min-width: 0;
	}

	.name {
		display: block;
		font-size: 15px;
		font-weight: 600;
	}

	.note {
		display: block;
		margin-top: 2px;
		font-size: 12px;
		color: var(--text-meta);
	}

	.mono {
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.08em;
		color: var(--text-secondary);
	}

	.later .mono {
		color: var(--text-meta);
	}

	@media (max-width: 720px) {
		.card {
			padding: 24px;
			border-radius: var(--radius-24);
		}
	}
</style>
