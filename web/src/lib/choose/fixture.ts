/**
 * Choose artwork on the fixtures: Low Tide Choir's candidates from fanart.tv and Commons, drawn here
 * from flat shapes as the boards' are (no real artist's picture), what its folder holds for Ana
 * Lindqvist, and every state the picker can meet: `?artwork=pinned` (its background pinned by
 * Simon), `failed` (fanart.tv answering 503), `off` (lookups off), `held` (Not this artist already
 * said). `?artworkMs=` holds the candidates back that long, as a first open fetching previews does.
 */
import { drawPictures } from '#lib/art/art.ts';
import type { ArtFixture, FixturePin } from '#lib/library/artFixture.ts';
import type { ExtraImage, Picture } from '#lib/library/source.ts';
import {
	ArtworkError,
	type ArtistImageCandidates,
	type ArtistImageKind,
	type ArtworkSource,
	type CandidateSource,
	type ImageCandidate
} from './types.ts';

export type ArtworkFixture = 'pinned' | 'failed' | 'off' | 'held';

/** The `?artwork=` a fixtures page opens on. */
export function artworkFixture(value: string | null): ArtworkFixture | undefined {
	return value === 'pinned' || value === 'failed' || value === 'off' || value === 'held'
		? value
		: undefined;
}

const lowTideChoir = 'artist-low-tide-choir';
const pinner = 'Simon';
/** The most an upload may be, as the server takes it. */
const maxUpload = 32 << 20;
const uploadTypes = new Set(['image/jpeg', 'image/png', 'image/webp']);

function svg(width: number, height: number, body: string): string {
	const markup = `<svg xmlns="http://www.w3.org/2000/svg" width="${String(width)}" height="${String(height)}" viewBox="0 0 ${String(width)} ${String(height)}">${body}</svg>`;
	return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(markup)}`;
}

/** A stage of three figures under a sky: the boards' backdrops, by their colours. */
function backdrop(sky: [string, string, string], light: string, at: number, beams = false): string {
	const beam = beams
		? `<path d="M520 0 L620 0 L780 860 L700 860 Z" fill="#f3e9d8" fill-opacity="0.32"/><path d="M1380 0 L1460 0 L1220 860 L1150 860 Z" fill="#e04f6a" fill-opacity="0.34"/>`
		: `<circle cx="${String(at)}" cy="700" r="230" fill="${light}" fill-opacity="0.2"/><circle cx="${String(at)}" cy="700" r="140" fill="${light}"/>`;
	return svg(
		1920,
		1080,
		`<defs><linearGradient id="s" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="${sky[0]}"/><stop offset="0.5" stop-color="${sky[1]}"/><stop offset="0.75" stop-color="${sky[2]}"/></linearGradient></defs>
		<rect width="1920" height="1080" fill="url(#s)"/>${beam}
		<rect y="780" width="1920" height="300" fill="#0e1018"/>
		<g fill="#120c18">${[0, 1, 2, 3, 4]
			.map(
				(n) =>
					`<circle cx="${String(820 + n * 90)}" cy="690" r="22"/><ellipse cx="${String(820 + n * 90)}" cy="810" rx="36" ry="96"/>`
			)
			.join('')}</g>`
	);
}

/** A glow over red: the crowd. */
const crowd = svg(
	1920,
	1080,
	`<defs><radialGradient id="g" cx="0.5" cy="0.2" r="0.7"><stop offset="0" stop-color="#ff7a52"/><stop offset="0.5" stop-color="#8a1f2a"/><stop offset="1" stop-color="#2a0b12"/></radialGradient></defs><rect width="1920" height="1080" fill="url(#g)"/>`
);

/** A face for a thumb, in its own colours. */
function face(ground: string, skin: string, shirt: string, hair = '#2a1712'): string {
	return svg(
		512,
		512,
		`<rect width="512" height="512" fill="${ground}"/><circle cx="256" cy="196" r="108" fill="${hair}"/>
		<ellipse cx="256" cy="232" rx="74" ry="94" fill="${skin}"/><ellipse cx="256" cy="512" rx="176" ry="144" fill="${shirt}"/>`
	);
}

/** The wordmark in one colour, transparent. */
function wordmark(fill: string): string {
	return svg(
		800,
		310,
		`<text x="400" y="196" text-anchor="middle" font-family="Arial Black, Helvetica, sans-serif" font-weight="900" font-size="96" textLength="760" lengthAdjust="spacingAndGlyphs" fill="${fill}">LOW TIDE CHOIR</text>`
	);
}

/** A banner: the name across a strip of sky. */
const banner = svg(
	1000,
	185,
	`<defs><linearGradient id="b" x1="0" y1="0" x2="1" y2="0"><stop offset="0" stop-color="#1c1430"/><stop offset="1" stop-color="#b0453a"/></linearGradient></defs><rect width="1000" height="185" fill="url(#b)"/><text x="40" y="120" font-family="Arial Black, Helvetica, sans-serif" font-weight="900" font-size="72" fill="#f3e9d8">LOW TIDE CHOIR</text>`
);

/** A candidate's stand-in hash: 64 hex digits from its seed, apart from the art fixture's. */
function hashOf(seed: number): string {
	return `c${seed.toString(16).padStart(1, '0')}`.repeat(32).slice(0, 64);
}

interface Offer {
	id: string;
	source: 'fanart' | 'wikimedia';
	width: number;
	height: number;
	lang?: string;
	credit?: ImageCandidate['credit'];
	preview: Picture;
}

function offer(
	seed: number,
	url: string,
	o: Omit<Offer, 'preview'>,
	colors: { accent: string; ground: string },
	alpha = false
): readonly [Offer, string] {
	const long = Math.max(o.width, o.height);
	const scale = Math.min(1, 400 / long);
	return [
		{
			...o,
			preview: {
				hash: hashOf(seed),
				width: Math.round(o.width * scale),
				height: Math.round(o.height * scale),
				alpha,
				...colors
			}
		},
		url
	];
}

const fanartPage = { pageUrl: 'https://fanart.tv/artist/low-tide-choir/' };

/** What the sources list for Low Tide Choir, by kind, in the order the pass would choose them. */
const offers: Record<ArtistImageKind, readonly (readonly [Offer, string])[]> = {
	background: [
		offer(
			1,
			backdrop(['#1c1430', '#4a2347', '#ee9a48'], '#f6c25a', 1344),
			{
				id: 'fanart:bg-dusk',
				source: 'fanart',
				width: 3840,
				height: 2160,
				lang: 'en',
				credit: fanartPage
			},
			{ accent: '#ee9a48', ground: '#2a1a33' }
		),
		offer(
			2,
			backdrop(['#140f22', '#2a1840', '#3a1a3a'], '#ffffff', 0, true),
			{ id: 'fanart:bg-stage', source: 'fanart', width: 1920, height: 1080, credit: fanartPage },
			{ accent: '#e04f6a', ground: '#1a1226' }
		),
		offer(
			3,
			backdrop(['#0d1730', '#1d3d63', '#25406e'], '#e7eef6', 760),
			{ id: 'fanart:bg-night', source: 'fanart', width: 1920, height: 1080, credit: fanartPage },
			{ accent: '#e7eef6', ground: '#0d1730' }
		),
		offer(
			4,
			crowd,
			{ id: 'fanart:bg-crowd', source: 'fanart', width: 1280, height: 720, credit: fanartPage },
			{ accent: '#ff7a52', ground: '#2a0b12' }
		)
	],
	thumb: [
		offer(
			5,
			face('#4a2347', '#f2c9a0', '#f3d27a'),
			{ id: 'fanart:th-choir', source: 'fanart', width: 1000, height: 1000, credit: fanartPage },
			{ accent: '#f3d27a', ground: '#4a2347' }
		),
		offer(
			6,
			face('#1f3a4a', '#c98f63', '#9fd3c7', '#3a2a1c'),
			{
				id: 'wikimedia:Low_Tide_Choir_2019.jpg',
				source: 'wikimedia',
				width: 1024,
				height: 1024,
				credit: {
					author: 'K. Arnesen',
					license: 'CC BY-SA 4.0',
					licenseUrl: 'https://creativecommons.org/licenses/by-sa/4.0/',
					pageUrl: 'https://commons.wikimedia.org/wiki/File:Low_Tide_Choir_2019.jpg'
				}
			},
			{ accent: '#9fd3c7', ground: '#1f3a4a' }
		),
		offer(
			7,
			face('#6e2a1c', '#e7b98f', '#e04f6a'),
			{ id: 'fanart:th-rose', source: 'fanart', width: 1000, height: 1000, credit: fanartPage },
			{ accent: '#e04f6a', ground: '#6e2a1c' }
		)
	],
	logo: [
		offer(
			8,
			wordmark('#f3e9d8'),
			{ id: 'fanart:lg-cream', source: 'fanart', width: 800, height: 310, credit: fanartPage },
			{ accent: '#f3e9d8', ground: '#2a1a33' },
			true
		),
		offer(
			9,
			wordmark('#f1a93b'),
			{ id: 'fanart:lg-fox', source: 'fanart', width: 800, height: 310, credit: fanartPage },
			{ accent: '#f1a93b', ground: '#2a1a33' },
			true
		),
		offer(
			10,
			wordmark('#9fd3c7'),
			{ id: 'fanart:lg-teal', source: 'fanart', width: 400, height: 155, credit: fanartPage },
			{ accent: '#9fd3c7', ground: '#2a1a33' },
			true
		)
	],
	banner: [
		offer(
			11,
			banner,
			{ id: 'fanart:bn-sky', source: 'fanart', width: 1000, height: 185, credit: fanartPage },
			{ accent: '#f3e9d8', ground: '#1c1430' }
		)
	]
};

/** Which candidate the art fixture's own images are, so the chosen one is ringed in the list. */
const shownAs = new Map<string, string>([
	['11'.repeat(32), 'fanart:th-choir'],
	['12'.repeat(32), 'fanart:bg-dusk'],
	['13'.repeat(32), 'fanart:lg-cream']
]);

export interface ArtworkFixtureOptions {
	art: ArtFixture;
	mode: ArtworkFixture | undefined;
	/** How long the candidates take. */
	candidatesMs: number;
	/** An answer as the fixtures give one: latency, and an administrator's only. */
	respond: <T>(answer: () => T) => Promise<T>;
	/** Whether the artist is known, and its MBID. */
	mbid: (artistId: string) => string | undefined;
	exists: (artistId: string) => boolean;
}

/** Choose artwork's routes over the fixtures. */
export function createArtworkFixture(o: ArtworkFixtureOptions): ArtworkSource {
	drawPictures(
		Object.values(offers)
			.flat()
			.map(([each, url]) => [each.preview.hash, url] as const)
	);
	if (o.mode === 'held') {
		o.art.reject(lowTideChoir);
	}
	if (o.mode === 'pinned') {
		const [stage] = offers.background[1] ?? [];
		if (stage !== undefined) {
			o.art.pin(lowTideChoir, 'background', {
				image: {
					...stage.preview,
					width: stage.width,
					height: stage.height,
					source: 'pinned',
					...(stage.credit === undefined ? {} : { credit: stage.credit })
				},
				candidate: stage.id,
				from: 'fanart',
				by: pinner,
				at: new Date(Date.now() - 3_600_000).toISOString()
			});
		}
	}
	let uploads = 0;

	function known(artistId: string): void {
		if (!o.exists(artistId)) {
			throw new ArtworkError(404, 'no such artist');
		}
	}

	/** How the sources stand for one artist. */
	function sources(artistId: string): CandidateSource[] {
		const each = (source: CandidateSource['source']): CandidateSource => {
			if (o.art.held(artistId)) {
				return { source, state: 'held' };
			}
			if (o.mode === 'off') {
				return { source, state: 'off' };
			}
			if (o.mbid(artistId) === undefined) {
				return { source, state: 'unavailable' };
			}
			if (o.mode === 'failed' && source === 'fanart') {
				return { source, state: 'failed', words: 'fanart.tv answered 503' };
			}
			return { source, state: 'answered' };
		};
		return [each('fanart'), each('wikimedia')];
	}

	/** The candidate a picture Kitsune holds is: a folder's, a source's, an upload. */
	function held(image: ExtraImage, chosen: boolean): ImageCandidate {
		const preview: Picture = {
			hash: image.hash,
			accent: image.accent,
			ground: image.ground,
			width: image.width,
			height: image.height,
			alpha: image.alpha
		};
		const source =
			image.source === 'folder' ? 'folder' : image.source === 'wikimedia' ? 'wikimedia' : 'fanart';
		return {
			id:
				source === 'folder'
					? 'folder'
					: (shownAs.get(image.hash) ?? `${source}:${image.hash.slice(0, 8)}`),
			source,
			width: image.width,
			height: image.height,
			preview,
			chosen,
			pinned: false,
			...(image.credit === undefined ? {} : { credit: image.credit })
		};
	}

	function listed(artistId: string, kind: ArtistImageKind): ImageCandidate[] {
		const out: ImageCandidate[] = [];
		const pin = o.art.pinOf(artistId, kind);
		if (pin !== undefined) {
			out.push({
				id: pin.candidate,
				source: pin.from,
				width: pin.image.width,
				height: pin.image.height,
				preview: {
					hash: pin.image.hash,
					accent: pin.image.accent,
					ground: pin.image.ground,
					width: pin.image.width,
					height: pin.image.height,
					alpha: pin.image.alpha
				},
				chosen: true,
				pinned: true,
				pinnedBy: pin.by,
				pinnedAt: pin.at,
				...(pin.image.credit === undefined ? {} : { credit: pin.image.credit })
			});
		}
		const automatic = o.art.automatic(artistId, kind);
		if (automatic !== undefined) {
			out.push(held(automatic, pin === undefined));
		}
		const answered = new Set(
			sources(artistId)
				.filter(({ state }) => state === 'answered')
				.map(({ source }) => source)
		);
		if (artistId === lowTideChoir) {
			for (const [each] of offers[kind]) {
				if (answered.has(each.source)) {
					out.push({ ...each, chosen: false, pinned: false });
				}
			}
		}
		// Once each, the first saying it: a pin, then what is shown, then the sources' order.
		return out.filter((c, at) => out.findIndex((other) => other.id === c.id) === at);
	}

	function pinWith(artistId: string, kind: ArtistImageKind, pin: FixturePin): ExtraImage {
		o.art.pin(artistId, kind, pin);
		return pin.image;
	}

	return {
		async candidates(artistId, kind) {
			if (o.candidatesMs > 0) {
				await new Promise((resolve) => setTimeout(resolve, o.candidatesMs));
			}
			return o.respond((): ArtistImageCandidates => {
				known(artistId);
				const mbid = o.mbid(artistId);
				return {
					kind,
					...(mbid === undefined ? {} : { mbid }),
					pinned: o.art.pinOf(artistId, kind) !== undefined,
					held: o.art.held(artistId),
					candidates: listed(artistId, kind).slice(0, 24),
					sources: sources(artistId)
				};
			});
		},

		pin: (artistId, kind, id) =>
			o.respond(() => {
				known(artistId);
				const c = listed(artistId, kind).find((each) => each.id === id);
				if (c?.preview === undefined) {
					throw new ArtworkError(422, 'no candidate of that kind has that ID now');
				}
				const image: ExtraImage = {
					...c.preview,
					width: c.width,
					height: c.height,
					source: 'pinned',
					...(c.credit === undefined ? {} : { credit: c.credit })
				};
				return pinWith(artistId, kind, {
					image,
					candidate: c.id,
					from: c.source,
					by: pinner,
					at: new Date().toISOString()
				});
			}),

		unpin: (artistId, kind) =>
			o.respond(() => {
				known(artistId);
				if (o.art.pinOf(artistId, kind) !== undefined) {
					o.art.pin(artistId, kind, undefined);
				}
			}),

		upload: (artistId, kind, file) =>
			o.respond(() => {
				known(artistId);
				if (!uploadTypes.has(file.type)) {
					throw new ArtworkError(415, 'the image must be JPEG, PNG or WebP');
				}
				if (file.size > maxUpload) {
					throw new ArtworkError(413, 'the image is larger than 32 MiB');
				}
				uploads += 1;
				const hash = `d${uploads.toString(16)}`.repeat(32).slice(0, 64);
				drawPictures([[hash, URL.createObjectURL(file)]]);
				const square = kind === 'thumb';
				const image: ExtraImage = {
					hash,
					width: square ? 1000 : 1920,
					height: square ? 1000 : 1080,
					accent: '#f28c28',
					ground: '#1a1410',
					alpha: file.type === 'image/png' && kind === 'logo',
					source: 'pinned'
				};
				return pinWith(artistId, kind, {
					image,
					candidate: `upload:${hash}`,
					from: 'upload',
					by: pinner,
					at: new Date().toISOString()
				});
			}),

		refresh: (artistId) =>
			o.respond(() => {
				known(artistId);
				if (o.mbid(artistId) === undefined) {
					throw new ArtworkError(409, 'the artist has no MusicBrainz ID');
				}
				if (o.mode === 'off') {
					throw new ArtworkError(409, 'artist lookups are off');
				}
				if (o.art.held(artistId)) {
					throw new ArtworkError(409, 'Not this artist holds it back');
				}
			}),

		reject: (artistId) =>
			o.respond(() => {
				known(artistId);
				if (o.mbid(artistId) === undefined) {
					throw new ArtworkError(409, 'the artist has no MusicBrainz ID');
				}
				o.art.reject(artistId);
			})
	};
}
