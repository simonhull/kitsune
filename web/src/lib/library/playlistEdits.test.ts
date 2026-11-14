import { describe, expect, it } from 'vitest';
import { movedTo, putBackInto, recount, takenFrom } from './playlistEdits.ts';
import type { PlaylistDetail, PlaylistEntry } from './source.ts';

const entry = (id: string, track = id, albumId = 'a'): PlaylistEntry => ({
	id,
	albumId,
	track: { id: track, title: track, durationMs: 1000, artists: [] }
});

const ids = (entries: readonly PlaylistEntry[]): string[] => entries.map(({ id }) => id);

const five = ['1', '2', '3', '4', '5'].map((id) => entry(id));

describe('takenFrom and putBackInto', () => {
	it('takes entries out and puts each run back before the entry that followed it', () => {
		const { left, taken } = takenFrom(five, ['2', '3', '5']);
		expect(ids(left)).toEqual(['1', '4']);
		expect(taken.runs.map(({ before, entries }) => [before, ids(entries)])).toEqual([
			['4', ['2', '3']],
			[undefined, ['5']]
		]);
		expect(ids(putBackInto(left, taken))).toEqual(['1', '2', '3', '4', '5']);
	});

	it('puts a run back at the end when the entry after it has gone too', () => {
		const { left, taken } = takenFrom(five, ['1']);
		expect(ids(putBackInto(left.slice(1), taken))).toEqual(['3', '4', '5', '1']);
	});
});

describe('movedTo', () => {
	it('tells the server where an entry went: after the one it follows, or before the first', () => {
		const down = movedTo(five, '1', 2);
		expect(down && ids(down.order)).toEqual(['2', '3', '1', '4', '5']);
		expect(down?.place).toEqual({ after: '3' });
		const top = movedTo(five, '4', 0);
		expect(top && ids(top.order)).toEqual(['4', '1', '2', '3', '5']);
		expect(top?.place).toEqual({ before: '1' });
		expect(movedTo(five, '5', 4)).toBeNull();
		expect(movedTo(five, 'gone', 1)).toBeNull();
	});

	it('moves one entry of a song in twice', () => {
		const twice = [entry('1', 's'), entry('2', 't'), entry('3', 's')];
		const moved = movedTo(twice, '3', 0);
		expect(moved?.order.map(({ track }) => track.id)).toEqual(['s', 's', 't']);
		expect(moved && ids(moved.order)).toEqual(['3', '1', '2']);
	});
});

describe('recount', () => {
	it('counts what the playlist holds now, and draws its mosaic from its albums in order', () => {
		const detail: PlaylistDetail = {
			id: 'p',
			name: 'P',
			source: 'kitsune',
			songCount: 0,
			albumCount: 0,
			durationMs: 0,
			mosaic: [],
			createdAt: '',
			updatedAt: '',
			entries: [entry('1', 'x', 'b'), entry('2', 'y', 'a'), entry('3', 'z', 'b')],
			albums: [
				{ id: 'a', title: 'A', artist: '', compilation: false },
				{ id: 'b', title: 'B', artist: '', compilation: false }
			]
		};
		const counted = recount(detail);
		expect([counted.songCount, counted.albumCount, counted.durationMs]).toEqual([3, 2, 3000]);
		expect(counted.mosaic.map(({ id }) => id)).toEqual(['b', 'a']);
	});
});
