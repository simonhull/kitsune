import { size } from './serverForm.ts';

const dayMs = 86_400_000;

/** At this many days left and under, a song in Trash says when it goes, in the danger color. */
export const dangerDays = 5;

/** What a toast says was moved: one song, an album's songs, or songs picked. */
export type TrashedWhat =
	| { kind: 'song'; title: string }
	| { kind: 'album'; title: string; songs: number }
	| { kind: 'songs'; songs: number };

/** A toast's words: the name in bold, when there is one, and what follows it. */
export interface ToastWords {
	name: string | null;
	rest: string;
}

function songs(count: number): string {
	return `${count.toLocaleString('en-US')} ${count === 1 ? 'song' : 'songs'}`;
}

/** "*Title* moved to Trash", "*Album* · 11 songs moved to Trash" or "3 songs moved to Trash". */
export function toastWords(what: TrashedWhat): ToastWords {
	switch (what.kind) {
		case 'song':
			return { name: what.title, rest: ' moved to Trash' };
		case 'album':
			return { name: what.title, rest: ` · ${songs(what.songs)} moved to Trash` };
		case 'songs':
			return { name: null, rest: `${songs(what.songs)} moved to Trash` };
	}
}

/** How many days a song has left in Trash: a part of a day counts as one, and the past as none. */
export function daysLeft(purgeAt: string, now: number): number {
	return Math.max(0, Math.ceil((Date.parse(purgeAt) - now) / dayMs));
}

/**
 * When a song in Trash goes, in the mono: "27 DAYS LEFT", then from {@link dangerDays} days "GONE IN
 * 4 DAYS", "GONE TOMORROW" and, once its day has come and the nightly pass hasn't, "GONE TODAY".
 */
export function leftWords(purgeAt: string, now: number): { text: string; danger: boolean } {
	const days = daysLeft(purgeAt, now);
	const danger = days <= dangerDays;
	if (!danger) {
		return { text: `${String(days)} DAYS LEFT`, danger };
	}
	if (days === 0) {
		return { text: 'GONE TODAY', danger };
	}
	return { text: days === 1 ? 'GONE TOMORROW' : `GONE IN ${String(days)} DAYS`, danger };
}

/** Midnight of the day `at` falls on, here. */
function dayOf(at: number): number {
	const day = new Date(at);
	day.setHours(0, 0, 0, 0);
	return day.getTime();
}

/**
 * When a song came into Trash and who moved it: "TODAY · 21:40 · SIMON", "YESTERDAY · 09:12 · SIMON"
 * and, from the day before, "3 DAYS AGO · SIMON". A song whose mover's account is gone says when only.
 */
export function whenWords(trashedAt: string, by: string | undefined, now: number): string {
	const at = Date.parse(trashedAt);
	const days = Math.round((dayOf(now) - dayOf(at)) / dayMs);
	const time = new Date(at).toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' });
	const when =
		days <= 0 ? `TODAY · ${time}` : days === 1 ? `YESTERDAY · ${time}` : `${String(days)} DAYS AGO`;
	return by === undefined || by === '' ? when : `${when} · ${by.toUpperCase()}`;
}

/** The line under the title: "6 SONGS · 186 MB · EACH ONE GOES FOR GOOD 30 DAYS AFTER IT CAME". */
export function summaryWords(
	trash: { count: number; bytes: number; retentionDays: number },
	short: boolean
): string {
	if (trash.count === 0) {
		return 'EMPTY';
	}
	const head = `${songs(trash.count).toUpperCase()} · ${size(trash.bytes)}`;
	const days = String(trash.retentionDays);
	return short
		? `${head} · ${days} DAYS EACH`
		: `${head} · EACH ONE GOES FOR GOOD ${days} DAYS AFTER IT CAME`;
}

/** An album's line in Trash: "FALLING UP · 3 SONGS". */
export function groupLine(artist: string, count: number): string {
	return `${artist.toUpperCase()} · ${songs(count).toUpperCase()}`;
}

/** An album's restore: "Restore" for its one song, "Restore 3" for more. */
export function restoreLabel(count: number): string {
	return count === 1 ? 'Restore' : `Restore ${count.toLocaleString('en-US')}`;
}
