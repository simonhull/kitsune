import type { ListedArtist } from './source.ts';
import { byKnown, byNumber, type ArtistOrder, type Sort } from './sorts.ts';

/**
 * Puts the Artists page in an order. The server sends the artists by sort name, ties by ID, so where
 * each stands is its name's rank, and every other order breaks its ties by that. Z to A turns the
 * names round, and keeps artists who share a name in their order.
 */
export function orderArtists(
	artists: readonly ListedArtist[],
	{ by, dir }: Sort<ArtistOrder>
): ListedArtist[] {
	if (by === 'name') {
		return dir === 'asc' ? [...artists] : turnNames(artists);
	}
	const value = (artist: ListedArtist): number | undefined => {
		switch (by) {
			case 'albums':
				return artist.albumCount;
			case 'plays':
				return artist.plays;
			case 'added': {
				const at = Date.parse(artist.addedAt);
				return Number.isNaN(at) ? undefined : at;
			}
		}
	};
	return artists
		.map((artist, rank) => ({ artist, rank, value: value(artist) }))
		.sort((a, b) =>
			by === 'added'
				? byKnown(a.value, b.value, dir) || a.rank - b.rank
				: byNumber(a.value ?? 0, b.value ?? 0, dir) || a.rank - b.rank
		)
		.map(({ artist }) => artist);
}

function turnNames(artists: readonly ListedArtist[]): ListedArtist[] {
	const runs: ListedArtist[][] = [];
	let last: string | undefined;
	for (const artist of artists) {
		const key = artist.name.toLowerCase();
		if (key === last) {
			runs.at(-1)?.push(artist);
		} else {
			runs.push([artist]);
			last = key;
		}
	}
	return runs.reverse().flat();
}

const month = new Intl.DateTimeFormat('en-US', {
	month: 'short',
	year: 'numeric',
	timeZone: 'UTC'
});

/** What stands beside an artist's name: what the list is sorted by, or its albums. */
export function artistFigure(artist: ListedArtist, { by }: Sort<ArtistOrder>): string {
	switch (by) {
		case 'plays':
			return `${artist.plays.toLocaleString('en-US')} ${artist.plays === 1 ? 'play' : 'plays'}`;
		case 'added': {
			const at = Date.parse(artist.addedAt);
			return Number.isNaN(at) ? '' : month.format(at);
		}
		case 'name':
		case 'albums':
			return artist.albumCount.toLocaleString('en-US');
	}
}
