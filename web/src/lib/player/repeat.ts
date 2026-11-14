import type { Repeat } from '#lib/library/source.ts';

export type { Repeat };

/**
 * What Repeat does now: an endless shuffle never comes to an end, so there `all` is off. The
 * setting stays, for the queue after it.
 */
export function repeatIn(repeat: Repeat, endless: boolean): Repeat {
	return endless && repeat === 'all' ? 'off' : repeat;
}

/** What a press of Repeat makes it: off, all, this song, off again. An endless shuffle skips all. */
export function cycled(repeat: Repeat, endless: boolean): Repeat {
	switch (repeatIn(repeat, endless)) {
		case 'off':
			return endless ? 'one' : 'all';
		case 'all':
			return 'one';
		case 'one':
			return 'off';
	}
}

/** Repeat's name, which says what it does now: "Repeat: off", "Repeat: all", "Repeat: this song". */
export function repeatLabel(repeat: Repeat, endless: boolean): string {
	switch (repeatIn(repeat, endless)) {
		case 'off':
			return endless ? 'Repeat: off (an endless shuffle repeats only a song)' : 'Repeat: off';
		case 'all':
			return 'Repeat: all';
		case 'one':
			return 'Repeat: this song';
	}
}
