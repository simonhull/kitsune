import { artworkUrl } from '#lib/library/api.ts';
import type { Player } from './player.svelte.ts';

/** The rung the system's own player shows. */
const artworkSize = 512;

/** Describes the track sounding, or null with nothing in the player. */
export function metadataOf({ current, queue }: Player): MediaMetadataInit | null {
	if (current === null || queue === null) {
		return null;
	}
	const { album } = queue;
	const hash = album.artwork?.hash;
	return {
		title: current.title,
		artist: current.artists.join(', ') || album.artist,
		album: album.title,
		artwork:
			hash === undefined
				? []
				: [
						{
							src: artworkUrl(hash, artworkSize),
							sizes: `${String(artworkSize)}x${String(artworkSize)}`,
							type: 'image/jpeg'
						}
					]
	};
}

export function playbackOf({ status }: Player): MediaSessionPlaybackState {
	return status === 'idle' ? 'none' : status === 'paused' ? 'paused' : 'playing';
}

export function positionOf({ current, position }: Player): MediaPositionState | undefined {
	const duration = (current?.durationMs ?? 0) / 1000;
	return duration > 0
		? { duration, position: Math.min(position, duration), playbackRate: 1 }
		: undefined;
}

/**
 * Keeps the system's media controls on what's actually sounding, reading the player directly: the
 * lock screen has no business waiting for a picture. Returns the cleanup.
 */
export function followMediaSession(
	player: Player,
	session: MediaSession | undefined = navigator.mediaSession
): () => void {
	if (session === undefined) {
		return () => undefined;
	}
	const actions: [MediaSessionAction, MediaSessionActionHandler][] = [
		[
			'play',
			() => {
				if (!player.sounding) {
					player.toggle();
				}
			}
		],
		[
			'pause',
			() => {
				player.pause();
			}
		],
		[
			'previoustrack',
			() => {
				player.previous();
			}
		],
		[
			'nexttrack',
			() => {
				player.next();
			}
		],
		[
			'seekto',
			({ seekTime }) => {
				if (seekTime !== undefined) {
					player.seek(seekTime);
				}
			}
		]
	];
	for (const [action, handler] of actions) {
		session.setActionHandler(action, handler);
	}

	const stop = $effect.root(() => {
		$effect(() => {
			const init = metadataOf(player);
			session.metadata = init === null ? null : new MediaMetadata(init);
		});
		$effect(() => {
			session.playbackState = playbackOf(player);
		});
		$effect(() => {
			session.setPositionState(positionOf(player));
		});
	});

	return () => {
		stop();
		for (const [action] of actions) {
			session.setActionHandler(action, null);
		}
	};
}
