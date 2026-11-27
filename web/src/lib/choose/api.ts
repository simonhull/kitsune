import type { ExtraImage } from '#lib/library/source.ts';
import { UnauthorizedError } from '#lib/library/source.ts';
import {
	ArtworkError,
	type ArtistImageCandidates,
	type ArtistImageKind,
	type ArtworkSource
} from './types.ts';

/** Choose artwork's routes over the real API, on `call`, which sends a write's request header. */
export function createArtworkApi(
	call: (path: string, init?: RequestInit) => Promise<Response>,
	onUnauthorized: () => void
): ArtworkSource {
	async function ask(path: string, init?: RequestInit): Promise<Response> {
		let response: Response;
		try {
			response = await call(path, init);
		} catch {
			throw new ArtworkError(0, '');
		}
		if (response.status === 401) {
			onUnauthorized();
			throw new UnauthorizedError();
		}
		if (!response.ok) {
			const problem = ((await response.json().catch(() => null)) ?? {}) as { detail?: string };
			throw new ArtworkError(response.status, problem.detail ?? '');
		}
		return response;
	}

	const artist = (id: string, then = ''): string =>
		`/admin/artists/${encodeURIComponent(id)}${then}`;
	const kindPath = (id: string, kind: ArtistImageKind, then = ''): string =>
		artist(id, `/images/${kind}${then}`);

	return {
		async candidates(id, kind) {
			const response = await ask(artist(id, `/images?kind=${kind}`));
			return (await response.json()) as ArtistImageCandidates;
		},

		async pin(id, kind, candidate) {
			const response = await ask(kindPath(id, kind), {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ candidate })
			});
			return (await response.json()) as ExtraImage;
		},

		async unpin(id, kind) {
			await ask(kindPath(id, kind), { method: 'DELETE' });
		},

		async upload(id, kind, file) {
			// The image itself, with its own media type: the server says what it takes.
			const response = await ask(kindPath(id, kind, '/upload'), {
				method: 'POST',
				headers: { 'Content-Type': file.type === '' ? 'application/octet-stream' : file.type },
				body: file
			});
			return (await response.json()) as ExtraImage;
		},

		async refresh(id) {
			await ask(artist(id, '/refresh'), { method: 'POST' });
		},

		async reject(id) {
			await ask(artist(id, '/reject'), { method: 'POST' });
		}
	};
}
