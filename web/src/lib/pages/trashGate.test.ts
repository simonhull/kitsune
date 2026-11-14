import { afterEach, describe, expect, it } from 'vitest';
import { library } from '#lib/library/library.svelte.ts';
import type { Me } from '#lib/library/source.ts';
import { actionsFor } from './songActions.ts';

const admin: Me = {
	id: 1,
	name: 'Simon',
	role: 'admin',
	createdAt: '2026-01-01T00:00:00Z',
	canEditTags: true,
	canTrash: true
};

const albumOf = new Map([['t1', 'a1']]);

describe('Move to Trash', () => {
	afterEach(() => {
		library.user = null;
	});

	it('is there for an administrator on a server that may change its files', () => {
		library.user = admin;
		expect(actionsFor(['t1'], albumOf).trash).toBeTypeOf('function');
	});

	it('is nowhere for anyone else, or while tag writes are off', () => {
		library.user = null;
		expect(actionsFor(['t1'], albumOf).trash).toBeUndefined();
		library.user = { ...admin, role: 'user', canEditTags: false, canTrash: false };
		expect(actionsFor(['t1'], albumOf).trash).toBeUndefined();
		library.user = { ...admin, canEditTags: false, canTrash: false };
		expect(actionsFor(['t1'], albumOf).trash).toBeUndefined();
	});
});
