/**
 * The fixtures' artist art, bios and album extras. Every picture is drawn here from flat shapes, as
 * the boards' are: no real artist's photo or logo. Low Tide Choir has everything, Ana Lindqvist only
 * what her folder holds, and everyone else nothing. With `late`, Low Tide Choir's art arrives only
 * when a test says so (`kitsune:art-arrives`), as an `artists` event would bring it.
 */
import { drawPictures } from '#lib/art/art.ts';
import type { AlbumExtras, ArtistBio, ArtistImages, ExtraImage } from './source.ts';

/** A picture's stand-in hash: 64 hex digits, one per fixture picture. */
function hashOf(seed: number): string {
	return seed.toString(16).padStart(2, '0').repeat(32);
}

function svg(width: number, height: number, body: string): string {
	const markup = `<svg xmlns="http://www.w3.org/2000/svg" width="${String(width)}" height="${String(height)}" viewBox="0 0 ${String(width)} ${String(height)}">${body}</svg>`;
	return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(markup)}`;
}

/** A jetty at dusk, three figures against the sun: 16:9. */
const dusk = svg(
	1920,
	1080,
	`<defs><linearGradient id="s" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#1c1430"/><stop offset="0.34" stop-color="#4a2347"/><stop offset="0.58" stop-color="#b0453a"/><stop offset="0.72" stop-color="#ee9a48"/></linearGradient><linearGradient id="w" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#2a2342"/><stop offset="1" stop-color="#0e1820"/></linearGradient></defs>
	<rect width="1920" height="1080" fill="url(#s)"/>
	<circle cx="1344" cy="713" r="230" fill="#f6c25a" fill-opacity="0.2"/>
	<circle cx="1344" cy="713" r="144" fill="#f6c25a"/>
	<rect y="756" width="1920" height="324" fill="url(#w)"/>
	<rect x="700" y="852" width="1230" height="26" fill="#140c1c"/>
	<g fill="#140c1c"><circle cx="1114" cy="664" r="20"/><ellipse cx="1114" cy="788" rx="36" ry="92"/><circle cx="1210" cy="648" r="20"/><ellipse cx="1210" cy="778" rx="36" ry="92"/><circle cx="1296" cy="670" r="20"/><ellipse cx="1296" cy="800" rx="36" ry="92"/></g>`
);

/** Three faces, shoulder to shoulder: square. */
const choir = svg(
	512,
	512,
	`<rect width="512" height="512" fill="#4a2347"/>
	<g fill="#2a1712"><circle cx="128" cy="170" r="92"/><circle cx="256" cy="150" r="100"/><circle cx="384" cy="170" r="92"/></g>
	<ellipse cx="128" cy="236" rx="62" ry="82" fill="#c98f63"/><ellipse cx="256" cy="226" rx="66" ry="88" fill="#f2c9a0"/><ellipse cx="384" cy="236" rx="62" ry="82" fill="#a86a45"/>
	<ellipse cx="128" cy="470" rx="120" ry="110" fill="#e04f6a"/><ellipse cx="256" cy="480" rx="130" ry="110" fill="#f3d27a"/><ellipse cx="384" cy="470" rx="120" ry="110" fill="#9fd3c7"/>`
);

/** A portrait by a window, long hair: square. */
const portrait = svg(
	512,
	512,
	`<rect width="512" height="512" fill="#1d3d63"/><rect x="360" y="60" width="110" height="200" fill="#d5e6f6" fill-opacity="0.18"/>
	<ellipse cx="236" cy="270" rx="120" ry="170" fill="#2a1712"/><ellipse cx="236" cy="230" rx="76" ry="96" fill="#e7b98f"/>
	<ellipse cx="236" cy="520" rx="190" ry="150" fill="#5fa8e8"/>`
);

/** One face for an artist whose folder has a portrait: square, in that artist's colors. */
function face(ground: string, skin: string, shirt: string): string {
	return svg(
		512,
		512,
		`<rect width="512" height="512" fill="${ground}"/><circle cx="256" cy="200" r="104" fill="#1c1a18"/>
		<ellipse cx="256" cy="236" rx="72" ry="92" fill="${skin}"/><ellipse cx="256" cy="510" rx="170" ry="140" fill="${shirt}"/>`
	);
}

/** The wordmark: transparent, wide. A heavy sans stands in for a fanart.tv HD logo. */
const logo = svg(
	800,
	310,
	`<text x="400" y="196" text-anchor="middle" font-family="Arial Black, Helvetica, sans-serif" font-weight="900" font-size="96" textLength="780" lengthAdjust="spacingAndGlyphs" fill="#f3e9d8">LOW TIDE CHOIR</text>`
);

/** Disc art: the disc alone, with its spindle hole, so what is behind shows through. */
function disc(field: string, band: string, accent: string): string {
	const ring = (r: number): string =>
		`M ${String(400 - r)} 400 a ${String(r)} ${String(r)} 0 1 0 ${String(2 * r)} 0 a ${String(r)} ${String(r)} 0 1 0 ${String(-2 * r)} 0 Z`;
	return svg(
		800,
		800,
		`<path fill-rule="evenodd" fill="${field}" d="${ring(398)} ${ring(52)}"/>
		<path fill-rule="evenodd" fill="${band}" d="${ring(290)} ${ring(256)}"/>
		<path fill-rule="evenodd" fill="${accent}" d="${ring(298)} ${ring(290)}"/>
		<path fill-rule="evenodd" fill="#e1e8ec" fill-opacity="0.42" d="${ring(120)} ${ring(52)}"/>
		<circle cx="400" cy="250" r="70" fill="${accent}"/>`
	);
}

/** A back cover: the songs as lines, a barcode on its white patch, the sun small. */
const back = svg(
	1024,
	1024,
	`<rect width="1024" height="1024" fill="#1f5c5e"/><rect y="860" width="1024" height="164" fill="#12393b"/>
	<g fill="#cde6e0" fill-opacity="0.7">${Array.from({ length: 12 }, (_, line) => `<rect x="140" y="${String(200 + line * 40)}" width="440" height="10"/>`).join('')}</g>
	<circle cx="870" cy="160" r="60" fill="#9fd3c7"/><rect x="760" y="830" width="220" height="130" fill="#f5f1ec"/>
	<g fill="#111">${Array.from({ length: 14 }, (_, bar) => `<rect x="${String(776 + bar * 14)}" y="846" width="${String(bar % 3 === 0 ? 7 : 4)}" height="80"/>`).join('')}</g>`
);

function picture(
	seed: number,
	url: string,
	size: { width: number; height: number },
	colors: { accent: string; ground: string },
	source: ExtraImage['source'],
	alpha = false,
	credit?: ExtraImage['credit']
): readonly [ExtraImage, string] {
	return [
		{
			hash: hashOf(seed),
			...size,
			...colors,
			alpha,
			source,
			...(credit === undefined ? {} : { credit })
		},
		url
	];
}

const square = { width: 512, height: 512 };
const fanart = { pageUrl: 'https://fanart.tv/artist/low-tide-choir/' };

const choirThumb = picture(
	0x11,
	choir,
	square,
	{ accent: '#f3d27a', ground: '#4a2347' },
	'fanart',
	false,
	fanart
);
const choirBackdrop = picture(
	0x12,
	dusk,
	{ width: 1920, height: 1080 },
	{ accent: '#ee9a48', ground: '#2a1a33' },
	'fanart',
	false,
	fanart
);
const choirLogo = picture(
	0x13,
	logo,
	{ width: 800, height: 310 },
	{ accent: '#f3e9d8', ground: '#2a1a33' },
	'fanart',
	true,
	fanart
);
const anaThumb = picture(
	0x21,
	portrait,
	square,
	{ accent: '#5fa8e8', ground: '#10243f' },
	'folder'
);
const glasshouseDisc = picture(
	0x31,
	disc('#1f5c5e', '#12393b', '#9fd3c7'),
	{ width: 800, height: 800 },
	{ accent: '#9fd3c7', ground: '#1f5c5e' },
	'fanart',
	true
);
const glasshouseBack = picture(
	0x32,
	back,
	{ width: 1024, height: 1024 },
	{ accent: '#9fd3c7', ground: '#1f5c5e' },
	'folder'
);
/** Portraits for a few more artists, so the Artists list shows faces beside initials. */
const faces = [
	[
		'artist-odile-marsh',
		picture(
			0x41,
			face('#6e2a1c', '#e7b98f', '#e8dcc4'),
			square,
			{ accent: '#e8dcc4', ground: '#6e2a1c' },
			'folder'
		)
	],
	[
		'artist-the-halden-set',
		picture(
			0x42,
			face('#2b2147', '#c98f63', '#f3d27a'),
			square,
			{ accent: '#f3d27a', ground: '#2b2147' },
			'folder'
		)
	],
	[
		'artist-mere-coast',
		picture(
			0x43,
			face('#1f5c5e', '#a86a45', '#9fd3c7'),
			square,
			{ accent: '#9fd3c7', ground: '#1f5c5e' },
			'folder'
		)
	]
] as const;

const lowTideChoir = 'artist-low-tide-choir';
const anaLindqvist = 'artist-ana-lindqvist';

const wikipedia: ArtistBio = {
	summary:
		'Low Tide Choir are a five-piece indie band from Bergen, Norway, formed in 2014. Built around layered voices and tape-warped synths, their records are named after weather and the sea.',
	body: [
		'Low Tide Choir are a five-piece indie band from Bergen, Norway, formed in 2014. Built around layered voices and tape-warped synths, their records are named after weather and the sea.',
		'The band began as a choir project at a music school in Bergen, singing harbour songs over drum machines in a disused ferry terminal.',
		'Glasshouse was made over one winter in a greenhouse outside Stavanger, which gave its songs their muffled, close sound.'
	].join('\n\n'),
	source: 'wikipedia',
	url: 'https://en.wikipedia.org/wiki/Low_Tide_Choir',
	license: 'CC BY-SA 4.0',
	licenseUrl: 'https://creativecommons.org/licenses/by-sa/4.0/',
	lang: 'en'
};

const nfo: ArtistBio = {
	summary:
		'Stockholm songwriter. Harbour Lights was recorded in one room over two weeks, with the windows open.',
	body: 'Stockholm songwriter. Harbour Lights was recorded in one room over two weeks, with the windows open.',
	source: 'nfo'
};

/** What the fixtures say of an artist's art, and how a late arrival is announced. */
export interface ArtFixture {
	images(artistId: string): ArtistImages;
	bio(artistId: string): ArtistBio | undefined;
	mbid(artistId: string): string | undefined;
	extras(albumId: string): AlbumExtras | undefined;
	/** Calls `heard` with the artists whose art arrived. Returns what stops it. */
	changed(heard: (ids: string[]) => void): () => void;
}

/** Draws the pictures and holds who has what. `late` keeps Low Tide Choir's for a test to bring. */
export function createArtFixture({ late = false }: { late?: boolean } = {}): ArtFixture {
	drawPictures(
		[
			choirThumb,
			choirBackdrop,
			choirLogo,
			anaThumb,
			glasshouseDisc,
			glasshouseBack,
			...faces.map(([, each]) => each)
		].map(([image, url]) => [image.hash, url] as const)
	);
	let arrived = !late;
	const listeners = new Set<(ids: string[]) => void>();
	if (late && typeof addEventListener === 'function') {
		addEventListener('kitsune:art-arrives', () => {
			arrived = true;
			for (const heard of listeners) {
				heard([lowTideChoir]);
			}
		});
	}
	const others = new Map<string, ExtraImage>(faces.map(([id, [image]]) => [id, image]));
	return {
		images: (artistId) => {
			if (artistId === lowTideChoir) {
				return arrived
					? { thumb: choirThumb[0], background: choirBackdrop[0], logo: choirLogo[0] }
					: {};
			}
			if (artistId === anaLindqvist) {
				return { thumb: anaThumb[0] };
			}
			const thumb = others.get(artistId);
			return thumb === undefined ? {} : { thumb };
		},
		bio: (artistId) =>
			artistId === lowTideChoir
				? arrived
					? wikipedia
					: undefined
				: artistId === anaLindqvist
					? nfo
					: undefined,
		mbid: (artistId) =>
			artistId === lowTideChoir ? '6a1b4c2e-0d5f-4c1a-9a3e-1f0b2c3d4e5f' : undefined,
		extras: (albumId) => {
			switch (albumId) {
				case 'glasshouse-66':
					return { discs: [{ disc: 0, image: glasshouseDisc[0] }], back: glasshouseBack[0] };
				case 'night-ferry-94':
					return { discs: [], coverSource: 'coverartarchive' };
				default:
					return { discs: [] };
			}
		},
		changed: (heard) => {
			listeners.add(heard);
			return () => listeners.delete(heard);
		}
	};
}
