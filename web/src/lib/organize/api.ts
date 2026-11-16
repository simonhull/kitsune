import { UnauthorizedError } from '#lib/library/source.ts';
import {
	OrganizeError,
	type OrganizeExample,
	type OrganizePreview,
	type OrganizeRun,
	type OrganizeRunDetail,
	type OrganizeSource
} from './types.ts';

/** The organize routes over the real API, on `call`, which sends a write's request header. */
export function createOrganizeApi(
	call: (path: string, init?: RequestInit) => Promise<Response>,
	onUnauthorized: () => void
): OrganizeSource {
	async function ask<T>(path: string, init?: RequestInit): Promise<T> {
		let response: Response;
		try {
			response = await call(path, init);
		} catch {
			throw new OrganizeError(0, '');
		}
		if (response.status === 401) {
			onUnauthorized();
			throw new UnauthorizedError();
		}
		if (!response.ok) {
			const problem = ((await response.json().catch(() => null)) ?? {}) as {
				detail?: string;
				field?: 'template' | 'compilations';
				at?: number;
			};
			const { detail = '', field, at } = problem;
			throw new OrganizeError(
				response.status,
				detail,
				field !== undefined && at !== undefined ? { field, at } : undefined
			);
		}
		return (await response.json()) as T;
	}

	const post = (body: unknown): RequestInit => ({
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(body)
	});
	const runPath = (id: string, then = ''): string =>
		`/admin/organize/runs/${encodeURIComponent(id)}${then}`;

	return {
		preview: (request) => ask<OrganizePreview>('/admin/organize/preview', post(request)),

		page(id, { filter, q, cursor }) {
			const query = new URLSearchParams({ filter });
			if (q !== undefined && q !== '') {
				query.set('q', q);
			}
			if (cursor !== undefined) {
				query.set('cursor', cursor);
			}
			return ask<OrganizePreview>(
				`/admin/organize/previews/${encodeURIComponent(id)}?${query.toString()}`
			);
		},

		async examples(count, seed) {
			const query = new URLSearchParams({ count: String(count), seed: String(seed) });
			const { songs } = await ask<{ songs: OrganizeExample[] }>(
				`/admin/organize/examples?${query.toString()}`
			);
			return songs;
		},

		async runs() {
			const { runs } = await ask<{ runs: OrganizeRun[] }>('/admin/organize/runs');
			return runs;
		},

		run: (id) => ask<OrganizeRunDetail>(runPath(id)),

		start: (request) => ask<OrganizeRun>('/admin/organize/runs', post(request)),

		stop: (id) => ask<OrganizeRun>(runPath(id, '/stop'), { method: 'POST' }),

		undo: (id) => ask<OrganizeRun>(runPath(id, '/undo'), { method: 'POST' })
	};
}
