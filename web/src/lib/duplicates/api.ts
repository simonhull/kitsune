import { UnauthorizedError } from '#lib/library/source.ts';
import {
	DuplicatesError,
	type DuplicateDetail,
	type DuplicateList,
	type DuplicatesSource,
	type MergePlan,
	type Merged
} from './types.ts';

/** The Duplicates routes over the real API, on `call`, which sends a write's request header. */
export function createDuplicatesApi(
	call: (path: string, init?: RequestInit) => Promise<Response>,
	onUnauthorized: () => void
): DuplicatesSource {
	async function send(path: string, init?: RequestInit): Promise<Response> {
		let response: Response;
		try {
			response = await call(path, init);
		} catch {
			throw new DuplicatesError(0, '');
		}
		if (response.status === 401) {
			onUnauthorized();
			throw new UnauthorizedError();
		}
		if (!response.ok) {
			const problem = ((await response.json().catch(() => null)) ?? {}) as {
				detail?: string;
				plan?: MergePlan;
			};
			throw new DuplicatesError(response.status, problem.detail ?? '', problem.plan);
		}
		return response;
	}

	const ask = async <T>(path: string, init?: RequestInit): Promise<T> =>
		(await (await send(path, init)).json()) as T;
	const group = (key: string, then = ''): string =>
		`/admin/duplicates/${encodeURIComponent(key)}${then}`;

	return {
		list(filter, cursor) {
			const query = new URLSearchParams({ filter });
			if (cursor !== undefined) {
				query.set('cursor', cursor);
			}
			return ask<DuplicateList>(`/admin/duplicates?${query.toString()}`);
		},

		detail: (key) => ask<DuplicateDetail>(group(key)),

		merge: (key, request) =>
			ask<Merged>(group(key, '/merge'), {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(request)
			}),

		async dismiss(key) {
			await send(group(key, '/dismiss'), { method: 'POST' });
		},

		async bringBack(key) {
			await send(group(key, '/dismiss'), { method: 'DELETE' });
		}
	};
}
