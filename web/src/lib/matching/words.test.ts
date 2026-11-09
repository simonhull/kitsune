import { describe, expect, it } from 'vitest';
import type { MatchEvidence, MatchMedium } from './types.ts';
import { chipOf, formatLine, scoreLook } from './words.ts';

const medium = (format: string): MatchMedium => ({ position: 1, format, title: '', trackCount: 9 });

describe('chipOf', () => {
	const cases: [MatchEvidence, string, string][] = [
		[
			{ kind: 'lengths', verdict: 'agrees', count: 17, of: 17 },
			'✓ 17/17 lengths agree',
			'17/17 lengths'
		],
		[
			{ kind: 'titleArtist', verdict: 'agrees' },
			'✓ Title and artist exact',
			'Title and artist exact'
		],
		[{ kind: 'tagged', verdict: 'agrees', count: 3 }, '✓ MBIDs tagged on 3 songs', 'MBIDs on 3'],
		[{ kind: 'discs', verdict: 'neutral', count: 2, of: 2 }, '· 2 discs agree', '2 discs agree'],
		[
			{ kind: 'discs', verdict: 'differs', count: 4, of: 2, format: '12" Vinyl' },
			'≠ 4 sides, not 2 discs',
			'4 sides'
		],
		[
			{ kind: 'discs', verdict: 'differs', count: 1, of: 2, format: 'CD' },
			'≠ 1 disc, not 2 discs',
			'1 disc'
		],
		[
			{ kind: 'songs', verdict: 'differs', count: 21, of: 17 },
			'≠ 4 songs you don’t have',
			'4 you don’t have'
		],
		[{ kind: 'songs', verdict: 'differs', count: 11, of: 17 }, '≠ 11 of 17 songs', '11 of 17'],
		[{ kind: 'artist', verdict: 'differs' }, '≠ Artist differs', 'artist differs'],
		[
			{ kind: 'lengths', verdict: 'neutral', count: 3, of: 11 },
			'· 3/11 lengths agree',
			'3/11 lengths'
		]
	];
	it.each(cases)('words %o', (evidence, long, short) => {
		const chip = chipOf(evidence);
		expect(`${chip.mark} ${chip.text}`).toBe(long);
		expect(chipOf(evidence, true).text).toBe(short);
	});
});

describe('scoreLook', () => {
	it('wears the fox from 60 up, and dims under 40', () => {
		expect(scoreLook(96)).toEqual({ fox: true, dim: false });
		expect(scoreLook(60)).toEqual({ fox: true, dim: false });
		expect(scoreLook(59)).toEqual({ fox: false, dim: false });
		expect(scoreLook(18)).toEqual({ fox: false, dim: true });
	});
});

describe('formatLine', () => {
	it('counts media of a format, and calls digital media Digital', () => {
		expect(formatLine([medium('CD'), medium('CD')])).toBe('2 × CD');
		expect(formatLine([medium('Digital Media'), medium('Digital Media')])).toBe('Digital');
		expect(formatLine([medium('12" Vinyl'), medium('12" Vinyl')])).toBe('2 × 12" Vinyl');
		expect(formatLine([medium('CD'), medium('DVD')])).toBe('CD + DVD');
		expect(formatLine([])).toBe('');
	});
});
