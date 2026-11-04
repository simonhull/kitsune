/**
 * How to go up from Now playing to an album. Now playing sits above the album on the path, so
 * this must never push a new entry: Back from the album would then go down again.
 *  - `back`: the album is the entry right behind us, so going up is going back.
 *  - `replace`: it isn't, so the album takes Now playing's place in the stack, and Back goes on
 *    up the path from there. A reload lands here too: nothing is known about what's behind us.
 */
export type AlbumExit = 'back' | 'replace';

/** Decides how to reach `albumPath`, given the page Now playing was entered from. */
export function albumExit(cameFrom: string | null, albumPath: string): AlbumExit {
	return cameFrom === albumPath ? 'back' : 'replace';
}
