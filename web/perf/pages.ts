/** What the tests know about Kitsune's pages: where things are, and how to get to them. */
import type { Page } from '@playwright/test';
import type { Manifest } from './library.ts';
import type { Wait } from './probe.ts';

/** An album as the Library shows it, with the first two songs the suite plays. */
export interface ListedAlbum {
	id: string;
	title: string;
	artist: string;
	/** The first song's ID: its row is what a test clicks. */
	firstTrack: string;
}

/** The Library's rows are on screen. */
export const libraryRows: Wait = { kind: 'selector', selector: 'main .row[data-track]' };

/** An open album page with its tracks and a decoded cover. */
export function albumPage(id: string): Wait {
	return {
		kind: 'loaded',
		selector: `main .album:has(.hero [data-cover-want]):has(section[aria-label="Tracks"] .row)`,
		image: `.hero .cover img:is(.ready, :not(.want))`,
		// The page for this album, not the one before it.
		path: `/album/${id}`
	};
}

/** A menu that has measured itself and is showing. */
export function menu(label?: string): Wait {
	return {
		kind: 'selector',
		selector: label === undefined ? '.floating.placed' : `.floating.placed[aria-label="${label}"]`
	};
}

export function playing(trackId: string): Wait {
	return { kind: 'playing', src: `/tracks/${trackId}/stream` };
}

export async function openLibrary(page: Page): Promise<void> {
	await page.goto('/library');
	await page.locator('main .row[data-track]').first().waitFor();
}

/** The albums the Library has mounted now, top to bottom. */
export function mountedAlbums(page: Page): Promise<ListedAlbum[]> {
	return page.evaluate(() =>
		[...document.querySelectorAll<HTMLElement>('main section.artist')].flatMap((artist) =>
			[...artist.querySelectorAll<HTMLElement>('section.album')].flatMap((album) => {
				const id = album.querySelector('a.art')?.getAttribute('href')?.split('/').pop();
				const firstTrack = album.querySelector<HTMLElement>('.row[data-track]')?.dataset['track'];
				return id === undefined || firstTrack === undefined
					? []
					: [
							{
								id,
								title: album.getAttribute('aria-label') ?? '',
								artist: artist.getAttribute('aria-label') ?? '',
								firstTrack
							}
						];
			})
		)
	);
}

/**
 * Finds the next album down the Library that has real audio and that `used` doesn't hold, scrolling
 * as it goes. The loud albums are the first in the Library's order.
 */
export async function nextLoudAlbum(
	page: Page,
	manifest: Manifest,
	used: Set<string>
): Promise<ListedAlbum> {
	const loud = new Set(manifest.loud.map(({ artist, title }) => `${artist}\u0000${title}`));
	for (let tries = 0; tries < 100; tries++) {
		const found = (await mountedAlbums(page)).find(
			(album) => !used.has(album.id) && loud.has(`${album.artist}\u0000${album.title}`)
		);
		if (found !== undefined) {
			used.add(found.id);
			return found;
		}
		await page.mouse.wheel(0, 600);
		await page.waitForTimeout(100);
	}
	throw new Error('no loud album left in the Library');
}

/** The songs of an album, in its order, from the API. */
export async function albumTracks(page: Page, id: string): Promise<string[]> {
	const response = await page.request.get(`/api/v1/albums/${id}`);
	const album = (await response.json()) as { tracks: { id: string }[] };
	return album.tracks.map((track) => track.id);
}
