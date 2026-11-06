import { describe, expect, it, vi } from 'vitest';
import { createFixtureSource, type FixtureEditFault } from '#lib/library/fixtures.ts';
import type { LibrarySource } from '#lib/library/source.ts';
import { editOf, type Tagger } from '#lib/library/tags.ts';
import { TagEdits, type TagSession } from './tagEdits.svelte.ts';

const id = 'glasshouse-66';
const first = `${id}-1`;
const second = `${id}-2`;

function taggerOn(source: LibrarySource): Tagger {
	return {
		albumTags: (album) => source.albumTags(album),
		editAlbum: async (album, etag, edit) => {
			await source.editAlbum(album, etag, edit);
		},
		reread: async (album) => {
			await source.album(album);
		}
	};
}

/** Opens the album, with its tags read. */
async function opened(editFault?: FixtureEditFault) {
	const source = createFixtureSource({ editFault });
	const edits = new TagEdits(taggerOn(source));
	edits.open(id);
	const session = edits.of(id) as TagSession;
	await vi.waitFor(() => {
		expect(session.tags).not.toBeNull();
	});
	return { source, edits, session };
}

describe('TagEdits', () => {
	it('opens an album once, and reads its tags', async () => {
		const { edits, session } = await opened();
		edits.open(id);
		expect(edits.of(id)).toBe(session);
		expect(session.tags?.tracks).toHaveLength(10);
	});

	it('lets go of an album left with nothing to lose', async () => {
		const { edits } = await opened();
		edits.depart(id);
		expect(edits.of(id)).toBeUndefined();
	});

	it('keeps unsaved edits through a return, on the files as they are then', async () => {
		const { source, edits, session } = await opened();
		session.setField(first, 'title', ['Mine']);
		edits.depart(id);
		expect(edits.of(id)).toBe(session);

		const { etag } = await source.album(id);
		await source.editAlbum(id, etag, { tracks: [{ id: second, set: { title: ['Theirs'] } }] });
		edits.arrive(id);
		await vi.waitFor(() => {
			expect(session.tags?.tracks.at(1)?.fields.title).toEqual(['Theirs']);
		});
		expect(session.tags && editOf(session.tags, session.draft)).toEqual({
			tracks: [{ id: first, set: { title: ['Mine'] } }]
		});
	});

	it('forgets everything for the next user', async () => {
		const { edits, session } = await opened();
		session.setField(first, 'title', ['Mine']);
		edits.reset();
		expect(edits.of(id)).toBeUndefined();
	});

	it('says why an album can not be edited, and keeps a draft through it', async () => {
		const source = createFixtureSource();
		const edits = new TagEdits(taggerOn(source));
		edits.open('nope');
		const session = edits.of('nope') as TagSession;
		await vi.waitFor(() => {
			expect(session.report?.retry).toBe(false);
		});
		expect(session.tags).toBeNull();
	});
});

describe('TagSession.write', () => {
	it('writes once and closes', async () => {
		const { source, edits, session } = await opened();
		session.setField(first, 'title', ['Mine']);
		session.setYear('1999');
		await expect(session.write()).resolves.toEqual({ kind: 'done' });
		expect(edits.of(id)).toBeUndefined();
		const album = await source.album(id);
		expect(album.tracks.at(0)?.title).toBe('Mine');
		expect(album.year).toBe(1999);
	});

	it('sends nothing with nothing to send', async () => {
		const { session } = await opened();
		await expect(session.write()).resolves.toEqual({ kind: 'idle' });
	});

	it('points at the first problem, and shows them all from then on', async () => {
		const { session } = await opened();
		session.setField(first, 'title', ['']);
		expect(session.problem(`title-${first}`)).toBeUndefined();
		await expect(session.write()).resolves.toEqual({
			kind: 'problem',
			field: `title-${first}`
		});
		expect(session.problem(`title-${first}`)).toBe('A track needs a title.');
	});

	it('shows a problem once its field has been left', async () => {
		const { session } = await opened();
		session.setYear('20');
		expect(session.problem('year')).toBeUndefined();
		session.leftField('year');
		expect(session.problem('year')).toBe('A year is four digits.');
	});

	it('keeps the draft through a 409, and the same edit then goes through', async () => {
		const { edits, session } = await opened('busy');
		session.setField(first, 'title', ['Mine']);
		await expect(session.write()).resolves.toEqual({ kind: 'stopped' });
		expect(session.report?.message).toContain('30 seconds');
		expect(session.report?.retry).toBe(true);
		expect(session.summary.changes).toBe(1);
		await expect(session.write()).resolves.toEqual({ kind: 'done' });
		expect(edits.of(id)).toBeUndefined();
	});

	it('carries a stale edit over to the files as they are now', async () => {
		const { session } = await opened('stale');
		const { etag } = session.tags ?? { etag: '' };
		session.setField(first, 'title', ['Mine']);
		await session.write();
		expect(session.report?.message).toContain('changed while you were editing');
		expect(session.tags?.etag).not.toBe(etag);
		expect(session.tags?.tracks.at(-1)?.fields.title).toEqual(['Moved by a scan']);
		expect(session.summary.changes).toBe(1);
		await expect(session.write()).resolves.toEqual({ kind: 'done' });
	});

	it('offers no retry a scan alone can fix', async () => {
		const { session } = await opened('drift');
		session.setField(first, 'title', ['Mine']);
		await session.write();
		expect(session.report?.message).toMatch(/outside Kitsune/);
		expect(session.report?.retry).toBe(false);
		await expect(session.write()).resolves.toEqual({ kind: 'idle' });
	});

	it('gives a 422 in the server’s words and keeps the draft', async () => {
		const { session } = await opened('refused');
		session.setField(first, 'title', ['Mine']);
		await session.write();
		expect(session.report?.detail).toContain('1066');
		expect(session.summary.changes).toBe(1);
	});

	it('reports a stop partway, and a retry finishes it', async () => {
		const { source, session } = await opened('partial');
		session.setField(first, 'title', ['One']);
		session.setField(second, 'title', ['Two']);
		await session.write();
		expect(session.report?.written).toEqual(['Breakwater']);
		expect(session.report?.unwritten).toEqual(['Salt in the Lungs']);
		expect(session.summary).toMatchObject({ changes: 1, only: second });
		await expect(session.write()).resolves.toEqual({ kind: 'done' });
		const album = await source.album(id);
		expect(album.tracks.slice(0, 2).map(({ title }) => title)).toEqual(['One', 'Two']);
	});
});
