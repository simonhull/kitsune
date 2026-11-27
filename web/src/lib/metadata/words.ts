/**
 * Health's Metadata card in words. A failure is always the server's own words: nothing here makes
 * up why a source stopped.
 */

import { count } from '#lib/library/home.ts';
import { whenWords } from '#lib/organize/words.ts';
import type {
	MetadataAlbums,
	MetadataArtists,
	MetadataPass,
	MetadataSource,
	MetadataSourceName,
	MetadataStatus
} from './types.ts';

/** What every lookup sends, and what it never does. */
export const sends = 'MUSICBRAINZ IDS, AND NOTHING ELSE';
export const never = 'NAMES, PATHS, PLAYS OR PEOPLE';

/** Whether any source is asked, now or when an artist falls due. */
function anyOn(status: MetadataStatus): boolean {
	return status.sources.some((source) => source.state === 'on' || source.state === 'resting');
}

/** The card's eyebrow: off, fetching, up to date, or every source stopped. */
export function eyebrowWords(status: MetadataStatus): string {
	if (!status.remote) {
		return 'ARTIST ART AND BIOS · OFF';
	}
	if (status.pass.running) {
		return 'ARTIST ART AND BIOS · FETCHING';
	}
	return anyOn(status) ? 'ARTIST ART AND BIOS · UP TO DATE' : 'ARTIST ART AND BIOS · NO SOURCE ON';
}

/** The offer's title: the artists a lookup can ask about, by their MBIDs. */
export function offerTitle(withMbid: number): string {
	if (withMbid <= 0) {
		return 'Fetch art and bios for your artists?';
	}
	return `Fetch art and bios for ${count(withMbid)} ${withMbid === 1 ? 'artist' : 'artists'}?`;
}

/** Where lookups go, as one line; a phone's is shorter. */
export function toWords(phone: boolean): string {
	return phone
		? 'MUSICBRAINZ, WIKIMEDIA, FANART.TV, LAST.FM, COVER ART ARCHIVE'
		: 'MUSICBRAINZ · WIKIPEDIA · WIKIDATA · COMMONS · FANART.TV · LAST.FM · COVER ART ARCHIVE';
}

/** The offer's keys row: fanart.tv's project key, which Kitsune doesn't ship. */
export function keysWords(fanart: MetadataSource | undefined, phone: boolean): string {
	if (fanart?.key === 'set') {
		return phone ? 'FANART.TV’S KEY IS SET' : 'FANART.TV’S KEY IS SET · SERVER › SETTINGS';
	}
	return phone ? 'FANART.TV NEEDS YOURS' : 'FANART.TV NEEDS YOURS · SERVER › SETTINGS';
}

/** The big number and what it counts: artists looked up while a pass goes, else those with art. */
export function figureWords(status: MetadataStatus): { figure: string; of: string } {
	const { artists } = status;
	if (status.pass.running) {
		return {
			figure: count(artists.lookedUp),
			of: `of ${count(artists.withMbid)} ${artists.withMbid === 1 ? 'artist' : 'artists'} looked up`
		};
	}
	return {
		figure: count(artists.withArt),
		of: `of ${count(artists.albumArtists)} ${artists.albumArtists === 1 ? 'artist has' : 'artists have'} art`
	};
}

/** How far the pass is, 0 to 1. */
export function passShare(pass: MetadataPass): number {
	return pass.total <= 0 ? 0 : Math.min(1, pass.done / pass.total);
}

/** Under the bar while a pass goes: the artist or album in hand and how far. */
export function nowWords(pass: MetadataPass, phone: boolean): string {
	const albums = pass.phase === 'albums';
	const far = `${count(pass.done)} OF ${count(pass.total)}${albums ? ' ALBUMS' : ''}`;
	const inHand = albums ? pass.album : pass.artist;
	const who = inHand === undefined ? '' : ` · ${inHand.name.toUpperCase()}`;
	return phone ? `NOW${who} · ${far}` : `NOW${who} · ${far} · MUSIC KEEPS PLAYING`;
}

/**
 * The albums' line: how many covers came from the Cover Art Archive, and how many albums still have
 * none, or null when there's nothing to say.
 */
export function albumsWords(albums: MetadataAlbums, phone: boolean): string | null {
	const parts: string[] = [];
	if (albums.fromArchive > 0) {
		const n = count(albums.fromArchive);
		parts.push(
			phone
				? `${n} ${albums.fromArchive === 1 ? 'COVER' : 'COVERS'} FROM THE ARCHIVE`
				: `${n} ALBUM ${albums.fromArchive === 1 ? 'COVER' : 'COVERS'} FROM THE COVER ART ARCHIVE`
		);
	}
	if (albums.withoutCover > 0 && !phone) {
		parts.push(
			`${count(albums.withoutCover)} ${albums.withoutCover === 1 ? 'ALBUM HAS' : 'ALBUMS HAVE'} NO COVER`
		);
	}
	return parts.length === 0 ? null : parts.join(' · ');
}

/** Under the bar once the pass is done: what found nothing, and when the next refresh falls due. */
export function restWords(
	artists: MetadataArtists,
	nextRefresh: string | undefined,
	phone: boolean
): string {
	const parts: string[] = [];
	if (artists.foundNothing > 0 && !phone) {
		parts.push(`${count(artists.foundNothing)} FOUND NOTHING YET · ASKED AGAIN LATER`);
	}
	if (nextRefresh !== undefined) {
		const day = new Date(nextRefresh)
			.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
			.toUpperCase();
		parts.push(`NEXT REFRESH ${day}`);
	}
	return parts.length === 0 ? 'EVERY ARTIST WITH AN ID ASKED ABOUT' : parts.join(' · ');
}

const names: Record<MetadataSourceName, string> = {
	musicbrainz: 'MusicBrainz',
	wikipedia: 'Wikipedia, Wikidata, Commons',
	fanarttv: 'fanart.tv',
	lastfm: 'Last.fm',
	coverartarchive: 'Cover Art Archive'
};

const whats: Record<MetadataSourceName, string> = {
	musicbrainz: 'who each artist and album is · no key',
	wikipedia: 'bios and portraits · no key',
	fanarttv: 'thumbs, backdrops, logos, banners and disc art',
	lastfm: 'bios Wikipedia lacks · your Last.fm key',
	coverartarchive: 'covers your files lack, back covers · no key'
};

export function sourceName(name: MetadataSourceName): string {
	return names[name];
}

/** What a source gives, and for fanart.tv whether a personal key brings it sooner. */
export function sourceWhat(source: MetadataSource): string {
	return source.name === 'fanarttv' && source.clientKey === 'set'
		? `${whats.fanarttv} · your own key too`
		: whats[source.name];
}

/** A source's state as its chip says it, a rest with the local time it ends. */
export function chipWords(source: MetadataSource): string {
	switch (source.state) {
		case 'on':
			return 'ON';
		case 'off':
			return 'OFF';
		case 'needsKey':
			return 'NEEDS A KEY';
		case 'refused':
			return 'REFUSED THE KEY';
		case 'resting': {
			if (source.until === undefined) {
				return 'RESTING';
			}
			const time = new Date(source.until).toLocaleTimeString('en-GB', {
				hour: '2-digit',
				minute: '2-digit'
			});
			return `RESTING · AGAIN ${time}`;
		}
	}
}

/** Whether a source's chip leads to its key in Server › Settings. */
export function chipLeadsToKey(source: MetadataSource): boolean {
	return source.state === 'needsKey' || source.state === 'refused';
}

/** Where a source's key is set: Last.fm's in its own section, fanart.tv's under Artist art. */
export function keyPlace(source: MetadataSource): '/server#lastfm' | '/server#art' {
	return source.name === 'lastfm' ? '/server#lastfm' : '/server#art';
}

/**
 * The source's last failure, in the server's words, and when: shown while it matters (refused,
 * resting, or on since), never for a source that's off or only waiting for its key.
 */
export function failureWords(source: MetadataSource, now: number): string | null {
	const { failure } = source;
	if (failure === undefined || source.state === 'off' || source.state === 'needsKey') {
		return null;
	}
	const said = `${failure.words} · ${whenWords(failure.at, now)}`;
	return source.state === 'on' ? `Last trouble: ${said}` : said;
}

/** The artists only Matching can give art, by giving their albums MusicBrainz IDs. */
export function noMbidWords(withoutMbid: number, phone: boolean): string {
	const who = `${count(withoutMbid)} ALBUM ${withoutMbid === 1 ? 'ARTIST HAS' : 'ARTISTS HAVE'} NO MUSICBRAINZ ID`;
	return phone ? `${who} · MATCH THEIR ALBUMS` : `${who} · MATCH THEIR ALBUMS TO FETCH THEIR ART`;
}
