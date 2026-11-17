import { UnauthorizedError } from '#lib/library/source.ts';
import {
	ReplayGainError,
	type GainRun,
	type GainRunDetail,
	type ReplayGainSource,
	type ReplayGainStatus
} from './types.ts';

/** The ReplayGain routes over the real API, on `call`, which sends a write's request header. */
export function createReplayGainApi(
	call: (path: string, init?: RequestInit) => Promise<Response>,
	onUnauthorized: () => void
): ReplayGainSource {
	async function ask<T>(path: string, init?: RequestInit): Promise<T> {
		let response: Response;
		try {
			response = await call(path, init);
		} catch {
			throw new ReplayGainError(0, '');
		}
		if (response.status === 401) {
			onUnauthorized();
			throw new UnauthorizedError();
		}
		if (!response.ok) {
			const problem = ((await response.json().catch(() => null)) ?? {}) as { detail?: string };
			throw new ReplayGainError(response.status, problem.detail ?? '');
		}
		return (await response.json()) as T;
	}

	const runPath = (id: string, then = ''): string =>
		`/admin/replaygain/runs/${encodeURIComponent(id)}${then}`;

	return {
		status: () => ask<ReplayGainStatus>('/admin/replaygain'),

		start: (albums) =>
			ask<GainRun>('/admin/replaygain/runs', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(albums === undefined ? {} : { albums })
			}),

		run: (id) => ask<GainRunDetail>(runPath(id)),

		stop: (id) => ask<GainRun>(runPath(id, '/stop'), { method: 'POST' }),

		undo: (id) => ask<GainRun>(runPath(id, '/undo'), { method: 'POST' })
	};
}
