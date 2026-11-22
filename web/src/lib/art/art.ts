/**
 * An artist's pictures and words, and an album's extras, as pages draw them: which rendition to ask
 * for, the initial that stands in for a thumb, and the credit lines. Pure, but for the one registry
 * the fixtures fill with pictures of their own.
 */
import { fnv1a, palettes } from '#lib/cover/cover.ts';
import { artworkUrl } from '#lib/library/api.ts';
import type {
	Album,
	ArtistBio,
	ArtistImages,
	DiscArt,
	ExtraImage,
	LibraryIndex
} from '#lib/library/source.ts';

/** What a picture is, which decides its ladder: `GET /artwork/{hash}`'s renditions per kind. */
export type PictureKind = 'thumb' | 'background' | 'logo' | 'disc' | 'back';

const ladders: Record<PictureKind, readonly number[]> = {
	thumb: [128, 512, 1024],
	back: [128, 512, 1024],
	background: [128, 640, 1920, 3840],
	logo: [128, 400, 800],
	disc: [128, 400, 800]
};

/** The rung for a picture drawn `pixels` across on the screen, density included: the smallest that covers it. */
export function pictureRung(kind: PictureKind, pixels: number): number {
	const ladder = ladders[kind];
	return ladder.find((rung) => rung >= pixels) ?? ladder.at(-1) ?? 512;
}

/** Pictures the fixtures draw themselves, by hash: no server holds them. */
const drawn = new Map<string, string>();

/** Lets the fixtures stand in for `GET /artwork/{hash}` with pictures of their own. */
export function drawPictures(pictures: Iterable<readonly [string, string]>): void {
	for (const [hash, url] of pictures) {
		drawn.set(hash, url);
	}
}

/** Every rendition decoded this session: one seen before shows at once, and never develops again. */
export const decodedPictures = new Set<string>();

/** Where a picture's rendition is: the server's, or one the fixtures drew. */
export function pictureUrl(hash: string, size: number): string {
	return drawn.get(hash) ?? artworkUrl(hash, size);
}

/** The letter an artist with no thumb is drawn as: the first letter or digit of the name. */
export function initialOf(name: string): string {
	const first = [...name.trim()].find((char) => /[\p{L}\p{N}]/u.test(char));
	return first === undefined ? '?' : first.toLocaleUpperCase();
}

/** The initial's own colors, the same for the artist everywhere: never a grey silhouette. */
export function initialColors(artistId: string): { ground: string; ink: string } {
	const palette = palettes[fnv1a(artistId) % palettes.length] ?? palettes[0];
	return palette === undefined
		? { ground: 'transparent', ink: 'currentColor' }
		: { ground: palette.field, ink: palette.accent };
}

/** A run of words in a credit line, a link where the source gives one. */
export interface CreditPart {
	text: string;
	href?: string;
}

const sourceNames: Partial<Record<ExtraImage['source'], string>> = {
	fanart: 'FANART.TV',
	audiodb: 'THEAUDIODB',
	wikimedia: 'COMMONS',
	coverartarchive: 'COVER ART ARCHIVE'
};

const kindWords: Record<'thumb' | 'background' | 'logo', string> = {
	thumb: 'PHOTO',
	background: 'BACKDROP',
	logo: 'LOGO'
};

/** Joins words the way every credit line does: `PHOTO AND BACKDROP`, `PHOTO, BACKDROP AND LOGO`. */
function listed(words: readonly string[]): string {
	return words.length <= 1
		? (words[0] ?? '')
		: `${words.slice(0, -1).join(', ')} AND ${words.at(-1) ?? ''}`;
}

/**
 * What the hero says of where its pictures came from: one line per source that asks to be named,
 * the author, the license and the page where the source gives them. The library's own files ask
 * nothing, so pictures from them say nothing.
 */
export function imageCredits(images: ArtistImages): CreditPart[][] {
	const kinds = ['thumb', 'background', 'logo'] as const;
	const bySource = new Map<string, { kinds: string[]; image: ExtraImage }[]>();
	for (const kind of kinds) {
		const image = images[kind];
		const name = image === undefined ? undefined : sourceNames[image.source];
		if (image === undefined || name === undefined) {
			continue;
		}
		const key = `${name}|${image.credit?.author ?? ''}|${image.credit?.license ?? ''}`;
		const group = bySource.get(key) ?? [];
		group.push({ kinds: [kindWords[kind]], image });
		bySource.set(key, group);
	}
	return [...bySource.values()].map((group) => {
		const { image } = group[0] ?? { image: undefined };
		const credit = image?.credit;
		const name = image === undefined ? '' : (sourceNames[image.source] ?? '');
		const parts: CreditPart[] = [{ text: listed(group.flatMap(({ kinds: each }) => each)) }];
		if (credit?.author !== undefined) {
			parts.push({ text: credit.author.toUpperCase() });
		}
		if (credit?.license !== undefined) {
			parts.push({
				text: credit.license.toUpperCase(),
				...(credit.licenseUrl === undefined ? {} : { href: credit.licenseUrl })
			});
		}
		parts.push(
			credit?.pageUrl === undefined ? { text: name } : { text: `${name} ›`, href: credit.pageUrl }
		);
		return parts;
	});
}

/** The paper tag a bio wears: it came from somewhere else. */
export function bioTag(source: ArtistBio['source']): string {
	switch (source) {
		case 'nfo':
			return 'YOUR FILES';
		case 'wikipedia':
			return 'WIKIPEDIA';
		case 'lastfm':
			return 'LAST.FM';
		case 'audiodb':
			return 'THEAUDIODB';
	}
}

/**
 * The bio's credit, in mono and always shown: where it came from, its license as a link, that it was
 * shortened, and the way to the whole of it. A bio from the library's own `artist.nfo` is whole.
 */
export function bioCredit(bio: ArtistBio): CreditPart[] {
	if (bio.source === 'nfo') {
		return [{ text: 'FROM ARTIST.NFO IN YOUR LIBRARY' }];
	}
	const from = { wikipedia: 'WIKIPEDIA', lastfm: 'LAST.FM', audiodb: 'THEAUDIODB' }[bio.source];
	const read = bio.source === 'wikipedia' ? 'READ THE ARTICLE ›' : `READ ON ${from} ›`;
	return [
		{ text: `FROM ${from}` },
		...(bio.license === undefined
			? []
			: [
					{
						text: bio.license.toUpperCase(),
						...(bio.licenseUrl === undefined ? {} : { href: bio.licenseUrl })
					}
				]),
		{ text: 'SHORTENED FROM THE ORIGINAL' },
		...(bio.url === undefined ? [] : [{ text: read, href: bio.url }])
	];
}

/** A bio's paragraphs past its summary, which More reveals: `body` splits on blank lines. */
export function bioRest({ summary, body }: ArtistBio): string[] {
	const paragraphs = body
		.split(/\n\s*\n/u)
		.map((paragraph) => paragraph.trim())
		.filter((paragraph) => paragraph !== '');
	return paragraphs.at(0) === summary.trim() ? paragraphs.slice(1) : paragraphs;
}

/** The art for the disc a song is on: its own disc's, else every disc's. */
export function discArtFor(
	album: Pick<Album, 'extras'>,
	discNo: number | undefined
): DiscArt | undefined {
	const discs = album.extras?.discs ?? [];
	return discs.find(({ disc }) => disc === (discNo ?? 1)) ?? discs.find(({ disc }) => disc === 0);
}

/** One picture in an album's Artwork strip, with where it came from. */
export interface StripPicture {
	key: string;
	label: string;
	/** Where it came from, in mono. */
	from: string;
	/** Absent for the front: the album's own cover draws it. */
	image?: ExtraImage;
}

function fromWords(source: ExtraImage['source']): string {
	return source === 'folder' ? 'IN THE FILES' : (sourceNames[source] ?? 'CHOSEN');
}

/**
 * The Artwork strip after an album's songs: its front, back and each disc, with where each came
 * from. An album with nothing beside its cover has no strip.
 */
export function artworkStrip(album: Pick<Album, 'extras' | 'artwork'>): StripPicture[] {
	const extras = album.extras;
	if (extras === undefined || (extras.back === undefined && extras.discs.length === 0)) {
		return [];
	}
	const front: StripPicture = {
		key: 'front',
		label: 'FRONT',
		from:
			album.artwork === undefined
				? 'DRAWN BY KITSUNE'
				: extras.coverSource === 'coverartarchive'
					? 'COVER ART ARCHIVE'
					: 'IN THE FILES'
	};
	const back: StripPicture[] =
		extras.back === undefined
			? []
			: [{ key: 'back', label: 'BACK', from: fromWords(extras.back.source), image: extras.back }];
	const discs = extras.discs.map(({ disc, image }) => ({
		key: `disc-${String(disc)}`,
		label: disc === 0 ? 'DISC' : `DISC ${String(disc)}`,
		from: fromWords(image.source),
		image
	}));
	return [front, ...back, ...discs];
}

/** The strip's count line: `4 PICTURES · 2 DISCS`. */
export function stripCount(pictures: readonly StripPicture[]): string {
	const discs = pictures.filter(({ key }) => key.startsWith('disc')).length;
	const count = `${String(pictures.length)} PICTURES`;
	return discs === 0 ? count : `${count} · ${String(discs)} ${discs === 1 ? 'DISC' : 'DISCS'}`;
}

/** The picture an artist's stage is drawn from on a screen this wide or wider: the backdrop. */
export function stagePicture(images: ArtistImages): ExtraImage | undefined {
	return images.background;
}

/**
 * The phone's stage: the thumb, square, since a 16:9 backdrop crops to a sliver under 431 px; the
 * backdrop where there is no thumb.
 */
export function phoneStagePicture(images: ArtistImages): ExtraImage | undefined {
	return images.thumb ?? images.background;
}

/** Whether an artist's page has a picture to stand on, at any width. */
export function hasStagePicture(images: ArtistImages): boolean {
	return images.background !== undefined || images.thumb !== undefined;
}

/**
 * The album whose stage an artist's page stands on when it has no picture: the most played of its
 * own, ties in the server's order (newest first), so a cold library takes the newest.
 */
export function fallbackAlbum(
	index: LibraryIndex | undefined,
	owned: readonly string[]
): string | undefined {
	let best: { id: string; plays: number } | undefined;
	for (const album of index?.artists.flatMap((artist) => artist.albums) ?? []) {
		if (!owned.includes(album.id)) {
			continue;
		}
		const plays = album.tracks.reduce((sum, track) => sum + (track.plays ?? 0), 0);
		if (best === undefined || plays > best.plays) {
			best = { id: album.id, plays };
		}
	}
	return best?.id;
}
