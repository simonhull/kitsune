import type { MatchCandidate, MatchEvidence, MatchMedium, MatchRelease } from './types.ts';

/** A chip of evidence, as the boards draw it: ✓ agrees, · is a fact, ≠ differs. */
export interface Chip {
	mark: '✓' | '·' | '≠';
	text: string;
	verdict: MatchEvidence['verdict'];
}

const marks = { agrees: '✓', neutral: '·', differs: '≠' } as const;

function songs(n: number): string {
	return `${String(n)} ${n === 1 ? 'song' : 'songs'}`;
}

/** Whether a format is played by sides, so its media are sides and not discs. */
function bySides(format = ''): boolean {
	return /vinyl|cassette|\blp\b/i.test(format);
}

/** Words for one piece of evidence. `short` is the phone's. */
export function chipOf(e: MatchEvidence, short = false): Chip {
	const count = e.count ?? 0;
	const of = e.of ?? 0;
	const words = (): string => {
		switch (e.kind) {
			case 'lengths':
				return short
					? `${String(count)}/${String(of)} lengths`
					: `${String(count)}/${String(of)} lengths agree`;
			case 'titleArtist':
				return 'Title and artist exact';
			case 'title':
				return 'Title differs';
			case 'artist':
				return short ? 'artist differs' : 'Artist differs';
			case 'tagged':
				return short ? `MBIDs on ${String(count)}` : `MBIDs tagged on ${songs(count)}`;
			case 'fingerprint':
				return `fingerprint ${String(count)}/${String(of)}`;
			case 'discs': {
				if (e.verdict !== 'differs') {
					return `${String(count)} discs agree`;
				}
				const unit = bySides(e.format) ? 'side' : 'disc';
				const media = `${String(count)} ${unit}${count === 1 ? '' : 's'}`;
				return short ? media : `${media}, not ${String(of)} ${of === 1 ? 'disc' : 'discs'}`;
			}
			case 'songs':
				if (count > of) {
					return short
						? `${String(count - of)} you don’t have`
						: `${songs(count - of)} you don’t have`;
				}
				return short ? `${String(count)} of ${String(of)}` : `${String(count)} of ${songs(of)}`;
		}
	};
	return { mark: marks[e.verdict], text: words(), verdict: e.verdict };
}

/** Confidence from here up wears the fox; below, grey. */
export const foxFrom = 60;
/** A release under this is dimmed. */
export const dimBelow = 40;

/** How a confidence is drawn. */
export function scoreLook(score: number): { fox: boolean; dim: boolean } {
	return { fox: score >= foxFrom, dim: score < dimBelow };
}

/** MusicBrainz's format names, as a person reads them. */
function formatName(format: string): string {
	return format === 'Digital Media' || format === '' ? 'Digital' : format;
}

/** A release's media as one line: "2 × CD", "Digital", "CD + DVD". */
export function formatLine(media: readonly MatchMedium[]): string {
	if (media.length === 0) {
		return '';
	}
	const groups: { format: string; count: number }[] = [];
	for (const { format } of media) {
		const name = formatName(format);
		const group = groups.find((g) => g.format === name);
		if (group === undefined) {
			groups.push({ format: name, count: 1 });
		} else {
			group.count++;
		}
	}
	return groups
		.map(({ format, count }) =>
			count === 1 || format === 'Digital' ? format : `${String(count)} × ${format}`
		)
		.join(' + ');
}

/** The release's label and catalog number, the first of each. */
export function labelOf(release: MatchRelease): { label: string; catalog: string } {
	const first = release.labels.at(0);
	return { label: first?.name ?? '', catalog: first?.catalogNumber ?? '' };
}

/** The facts beside a release's title, joined: date, country, format and songs. */
export function factsLine(release: MatchRelease, withArtist = false): string {
	return [
		withArtist ? release.artist : '',
		release.date ?? '',
		release.country ?? '',
		formatLine(release.media),
		songs(release.trackCount)
	]
		.filter((part) => part !== '')
		.join(' · ');
}

/** What the review's eyebrow says of the candidate: its score and its strongest agreements. */
export function reviewLine(c: MatchCandidate): string {
	const agreeing = c.evidence
		.filter(({ verdict, kind }) => verdict === 'agrees' && kind !== 'titleArtist')
		.map((e) => chipOf(e).text.toUpperCase());
	return [`${String(c.score)}%`, ...agreeing].join(' · ');
}
