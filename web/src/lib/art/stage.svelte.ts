import { paletteFor, type AlbumPalette } from '#lib/cover/palette.ts';
import { library } from '#lib/library/library.svelte.ts';
import type { ArtistImages } from '#lib/library/source.ts';
import { theme } from '#lib/theme/theme.svelte.ts';
import { fallbackAlbum, phoneStagePicture } from './art.ts';

/** An artist's stage: the hero's on their page, or the backdrop of Now playing. */
export interface ArtistBackdrop {
	/** Whose it is, so a picture of it is only ever carried on by the same artist's. */
	id: string;
	images: ArtistImages;
	/** The colours of the album the hero falls back on with no backdrop. */
	fallback: AlbumPalette | null;
	place: 'hero' | 'poster';
	/** Whether it has anything to show from 431 px up: a backdrop, or the album's stage. */
	wide: boolean;
	/** Whether it has anything to show under 431 px: the thumb, the backdrop, or the album's stage. */
	square: boolean;
}

/**
 * The hero's stage on an artist's page: their backdrop, else the stage of the album they play most
 * (once their songs say which); none with no picture and no albums. Null until the artist is read.
 */
export function heroStage(id: string): ArtistBackdrop | null {
	const load = library.artist(id);
	if (load.kind !== 'ready') {
		return null;
	}
	const { images, albums } = load.value;
	const songs = library.artistSongs(id);
	const lean =
		images.background === undefined && songs.kind === 'ready'
			? fallbackAlbum(
					songs.value,
					albums.map((album) => album.id)
				)
			: undefined;
	const fallback = lean === undefined ? null : paletteFor(library.entry(lean), theme.current);
	const wide = images.background !== undefined || fallback !== null;
	const square = phoneStagePicture(images) !== undefined || fallback !== null;
	return wide || square ? { id, images, fallback, place: 'hero', wide, square } : null;
}

/** Now playing's backdrop: the playing artist's, when they have one (decision 12). */
export function posterStage(id: string): ArtistBackdrop | null {
	const load = library.artist(id);
	if (load.kind !== 'ready' || load.value.images.background === undefined) {
		return null;
	}
	const { images } = load.value;
	return { id, images, fallback: null, place: 'poster', wide: true, square: true };
}
