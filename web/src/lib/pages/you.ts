import type { LastfmStatus, Listening, User } from '#lib/library/source.ts';

/** Which of the drawn states the Last.fm card is in. */
export type LastfmCard = 'unset' | 'none' | 'connected' | 'waiting' | 'reconnect';

/** How a return from last.fm went, as the server says in `?lastfm=`. */
export type ConnectResult = 'connected' | 'refused' | 'failed';

/** Picks the card's state from the connection. */
export function cardOf(status: LastfmStatus): LastfmCard {
	if (!status.configured) {
		return 'unset';
	}
	if (!status.connected) {
		return 'none';
	}
	return status.state === 'ok' ? 'connected' : status.state;
}

/** The mono line over the name. */
export function roleLine(role: User['role']): string {
	return role === 'admin' ? 'YOU · ADMINISTRATOR' : 'YOU · LISTENER';
}

function counted(n: number, one: string, many: string): string {
	return `${n.toLocaleString('en-US')} ${n === 1 ? one : many}`;
}

/** The counts under the name, as board A sets them. */
export function countsLine({ plays, rated }: Listening): string {
	return [counted(plays, 'PLAY', 'PLAYS'), `${counted(rated, 'SONG', 'SONGS')} RATED`].join(' · ');
}

/** "2 minutes ago", and the like, for something that happened at `then`. */
export function ago(then: number, now: number): string {
	const minutes = Math.floor((now - then) / 60_000);
	if (minutes < 1) {
		return 'just now';
	}
	if (minutes < 60) {
		return counted(minutes, 'minute', 'minutes') + ' ago';
	}
	const hours = Math.floor(minutes / 60);
	if (hours < 24) {
		return counted(hours, 'hour', 'hours') + ' ago';
	}
	return counted(Math.floor(hours / 24), 'day', 'days') + ' ago';
}

/** What the connected card says after the name. */
export function lastSentLine(lastSentAt: string | undefined, now: number): string {
	return lastSentAt === undefined
		? 'nothing sent yet'
		: `last scrobble ${ago(Date.parse(lastSentAt), now)}`;
}

/** When a waiting outbox goes next, as the states board says it. */
export function retryLine(retryAt: string | undefined, now: number): string {
	if (retryAt === undefined) {
		return 'TRYING AGAIN SOON';
	}
	const minutes = Math.ceil((Date.parse(retryAt) - now) / 60_000);
	return minutes <= 0 ? 'TRYING AGAIN NOW' : `TRYING AGAIN IN ${String(minutes)} MIN`;
}

/** "3 plays", for the waiting and reconnecting cards. */
export function playsWaiting(n: number): string {
	return counted(n, 'play', 'plays');
}

/** Reads `?lastfm=` from the URL the callback sent the browser to. */
export function resultOf(value: string | null): ConnectResult | null {
	return value === 'connected' || value === 'refused' || value === 'failed' ? value : null;
}

/** The one line a return from last.fm shows. */
export function resultLine(result: ConnectResult, name: string | undefined): string {
	switch (result) {
		case 'connected':
			return name === undefined ? 'Connected to Last.fm' : `Connected as ${name}`;
		case 'refused':
			return 'Not connected: you said no on last.fm.';
		case 'failed':
			return "Not connected: that didn't work. Try again.";
	}
}
