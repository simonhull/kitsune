import { describe, expect, it, vi } from 'vitest';
import { createFixtureSource, type FixtureEditFault } from '#lib/library/fixtures.ts';
import { Library } from '#lib/library/library.svelte.ts';
import { sharedFieldId } from '#lib/library/edit.ts';
import { TagEditor, type EditSession, type Opening } from './tagEditor.svelte.ts';

const glass = 'glasshouse-66';
const tide = 'night-ferry-94';
const first = `${glass}-1`;
const second = `${glass}-2`;

/** A library on the fixtures, with an editor over it. */
function editorOn(editFault?: FixtureEditFault) {
	const source = createFixtureSource({ editFault });
	const library = new Library(source, {
		events: null,
		readyMemory: { seen: () => true, mark: () => undefined }
	});
	return { source, library, editor: new TagEditor(library) };
}

/** Opens the editor, with everything it reads read. */
async function opened(opening: Opening, editFault?: FixtureEditFault, canTag = true) {
	const held = editorOn(editFault);
	const session = held.editor.open(opening, canTag);
	await vi.waitFor(() => {
		expect(session.rulesRead).toBe(true);
	});
	return { ...held, session };
}

const album = (more: Partial<Extract<Opening, { kind: 'album' }>> = {}): Opening => ({
	kind: 'album',
	albumId: glass,
	...more
});

const across: Opening = {
	kind: 'songs',
	songs: [
		{ id: first, albumId: glass, title: 'Breakwater' },
		{ id: `${tide}-1`, albumId: tide, title: 'Tidewrack' }
	]
};

function titles(session: EditSession): string[] {
	return session.tracks.map((track) => track.tags.fields.title?.at(0) ?? '');
}

describe('opening the editor', () => {
	it('opens an album with every song ticked', async () => {
		const { session } = await opened(album());
		expect(session.shape).toBe('album');
		expect(session.tracks).toHaveLength(10);
		expect(session.tickedIds).toHaveLength(10);
		expect(session.oneAlbum).toBe(glass);
	});

	it('opens a song’s menu on its album, that song alone ticked and open', async () => {
		const { session } = await opened(album({ ticked: [second], open: second }));
		expect(session.tracks).toHaveLength(10);
		expect(session.tickedIds).toEqual([second]);
		expect(session.open).toBe(second);
	});

	it('opens songs picked on one album as that album, only they ticked', async () => {
		const { session } = await opened({
			kind: 'songs',
			songs: [
				{ id: first, albumId: glass, title: 'One' },
				{ id: second, albumId: glass, title: 'Two' }
			]
		});
		expect(session.shape).toBe('album');
		expect(session.tickedIds).toEqual([first, second]);
	});

	it('opens songs across albums as a selection of just them', async () => {
		const { session } = await opened(across);
		expect(session.shape).toBe('selection');
		expect(session.tracks.map(({ id }) => id)).toEqual([first, `${tide}-1`]);
		expect(session.albumIds).toEqual([glass, tide]);
		expect(session.oneAlbum).toBeNull();
		session.toggle(`${tide}-1`);
		expect(session.oneAlbum).toBe(glass);
	});

	it('lists the songs as the library has them for someone who can’t change tags', async () => {
		const { session } = await opened(across, undefined, false);
		expect(session.shape).toBe('selection');
		expect(titles(session)).toEqual(['Breakwater', 'Tidewrack']);
		expect(session.albums).toBeNull();
	});

	it('drops the editor open before for a new one, and forgets it for the next user', async () => {
		const { editor, session } = await opened(album());
		const next = editor.open(album(), true);
		expect(editor.current).toBe(next);
		expect(next.key).not.toBe(session.key);
		editor.reset();
		expect(editor.current).toBeNull();
	});

	it('says why an album can’t be edited', async () => {
		const { editor } = editorOn();
		const session = editor.open({ kind: 'album', albumId: 'nope' }, true);
		await vi.waitFor(() => {
			expect(session.report?.retry).toBe(false);
		});
		expect(session.albums).toBeNull();
	});
});

describe('ticking', () => {
	it('ticks and unticks one, and all', async () => {
		const { session } = await opened(album());
		session.toggle(first);
		expect(session.tickedIds).toHaveLength(9);
		session.toggleAll();
		expect(session.tickedIds).toHaveLength(10);
		session.toggleAll();
		expect(session.tickedIds).toHaveLength(0);
	});

	it('gives a shared edit to the ticked songs only', async () => {
		const { library, session } = await opened(album());
		session.toggleAll();
		session.toggle(first);
		session.toggle(second);
		session.setShared('year', ['1999']);
		expect(session.summary).toMatchObject({ changes: 1, files: 2 });
		await expect(session.write()).resolves.toMatchObject({ kind: 'done', albumId: glass });
		const tags = await library.albumTags(glass);
		const years = tags.tracks.map(({ fields }) => fields.date?.at(0)?.slice(0, 4));
		expect(years.slice(0, 2)).toEqual(['1999', '1999']);
		expect(years.slice(2).every((year) => year !== '1999')).toBe(true);
	});

	it('writes a song’s own edit with the shared one, ticked or not', async () => {
		const { library, session } = await opened(album());
		session.toggle(first);
		session.setField(first, 'title', ['Mine']);
		session.setShared('genres', ['Noise']);
		await session.write();
		const [one, two] = (await library.albumTags(glass)).tracks;
		expect(one?.fields).toMatchObject({ title: ['Mine'] });
		expect(one?.fields.genres).not.toEqual(['Noise']);
		expect(two?.fields.genres).toEqual(['Noise']);
	});

	it('runs the quick jobs over the ticked songs only', async () => {
		const { session } = await opened(album());
		session.toggleAll();
		session.toggle(second);
		session.numberFromOne();
		expect(Object.keys(session.draft.tracks)).toEqual([second]);
		expect(session.draft.tracks[second]?.fields.track).toEqual(['1']);
	});
});

describe('where a write goes', () => {
	it('sends one album’s songs through its own route', async () => {
		const { library, session } = await opened(album());
		const editAlbum = vi.spyOn(library, 'editAlbum');
		const editTracks = vi.spyOn(library, 'editTracks');
		session.setField(first, 'title', ['Mine']);
		await session.write();
		expect(editAlbum).toHaveBeenCalledOnce();
		expect(editTracks).not.toHaveBeenCalled();
	});

	it('sends songs across albums through POST /edits', async () => {
		const { library, session } = await opened(across);
		const editAlbum = vi.spyOn(library, 'editAlbum');
		const editTracks = vi.spyOn(library, 'editTracks');
		session.setShared('label', ['Fern']);
		await expect(session.write()).resolves.toMatchObject({ kind: 'done' });
		expect(editAlbum).not.toHaveBeenCalled();
		expect(editTracks).toHaveBeenCalledOnce();
		expect(editTracks.mock.calls[0]?.[0].albums.map(({ id }) => id)).toEqual([glass, tide]);
	});

	it('sends a selection’s songs through one album’s route once only its songs change', async () => {
		const { library, session } = await opened(across);
		const editAlbum = vi.spyOn(library, 'editAlbum');
		session.setField(`${tide}-1`, 'title', ['Only this']);
		await session.write();
		expect(editAlbum).toHaveBeenCalledWith(tide, expect.any(String), expect.anything());
	});

	it('takes a match into the fields before anything is written', async () => {
		const { library, session } = await opened(album());
		const editAlbum = vi.spyOn(library, 'editAlbum');
		session.fill(glass, { tracks: [{ id: first, set: { title: ['From the match'] } }] });
		expect(editAlbum).not.toHaveBeenCalled();
		expect(session.summary).toMatchObject({ changes: 1, only: first });
		await expect(session.write()).resolves.toMatchObject({ kind: 'done', matched: glass });
	});
});

describe('writing', () => {
	it('sends nothing with nothing to send', async () => {
		const { session } = await opened(album());
		await expect(session.write()).resolves.toEqual({ kind: 'idle' });
	});

	it('points at the first problem, and shows them all from then on', async () => {
		const { session } = await opened(album());
		session.setField(first, 'title', ['']);
		expect(session.problem(`title-${first}`)).toBeUndefined();
		await expect(session.write()).resolves.toEqual({ kind: 'problem', field: `title-${first}` });
		expect(session.problem(`title-${first}`)).toBe('A song needs a title.');
	});

	it('shows a problem once its field has been left', async () => {
		const { session } = await opened(album());
		session.setShared('year', ['20']);
		expect(session.problem(sharedFieldId('year'))).toBeUndefined();
		session.leftField(sharedFieldId('year'));
		expect(session.problem(sharedFieldId('year'))).toBe('A year is four digits.');
	});

	it('keeps the draft through a 409, and the same edit then goes through', async () => {
		const { session } = await opened(album(), 'busy');
		session.setField(first, 'title', ['Mine']);
		await expect(session.write()).resolves.toEqual({ kind: 'stopped' });
		expect(session.report?.message).toContain('30 seconds');
		expect(session.summary.changes).toBe(1);
		await expect(session.write()).resolves.toMatchObject({ kind: 'done' });
	});

	it('carries a stale edit over to the files as they are now', async () => {
		const { session } = await opened(album(), 'stale');
		const etag = session.albums?.at(0)?.tags.etag;
		session.setField(first, 'title', ['Mine']);
		await session.write();
		expect(session.report?.message).toContain('changed while you were editing');
		expect(session.albums?.at(0)?.tags.etag).not.toBe(etag);
		expect(titles(session).at(-1)).toBe('Moved by a scan');
		expect(session.summary.changes).toBe(1);
		await expect(session.write()).resolves.toMatchObject({ kind: 'done' });
	});

	it('offers no retry a scan alone can fix', async () => {
		const { session } = await opened(album(), 'drift');
		session.setField(first, 'title', ['Mine']);
		await session.write();
		expect(session.report?.message).toMatch(/outside Kitsune/);
		await expect(session.write()).resolves.toEqual({ kind: 'idle' });
	});

	it('gives a 422 in the server’s words and keeps the draft', async () => {
		const { session } = await opened(album(), 'refused');
		session.setField(first, 'title', ['Mine']);
		await session.write();
		expect(session.report?.detail).toContain('1066');
		expect(session.summary.changes).toBe(1);
	});

	it('reports a stop partway, and a retry finishes it', async () => {
		const { library, session } = await opened(album(), 'partial');
		session.setField(first, 'title', ['One']);
		session.setField(second, 'title', ['Two']);
		await session.write();
		expect(session.report?.written).toEqual(['Breakwater']);
		expect(session.report?.unwritten).toEqual(['Salt in the Lungs']);
		expect(session.summary).toMatchObject({ changes: 1, only: second });
		await expect(session.write()).resolves.toMatchObject({ kind: 'done' });
		const tags = await library.albumTags(glass);
		expect(tags.tracks.slice(0, 2).map(({ fields }) => fields.title?.at(0))).toEqual([
			'One',
			'Two'
		]);
	});

	it('saves a shuffle switch with the tags, and one song’s own switch', async () => {
		const { library, session } = await opened(album());
		const changes = vi.spyOn(library, 'changeShuffleRules');
		session.setExcluded(first, true);
		expect(session.isExcluded(first)).toBe(true);
		expect(session.summary.changes).toBe(1);
		await expect(session.write()).resolves.toMatchObject({ kind: 'done' });
		expect(changes).toHaveBeenCalledWith([{ kind: 'exclude', tracks: [first], on: true }]);
	});

	it('lets the Shuffle tab’s switch speak for every ticked song', async () => {
		const { session } = await opened(album());
		session.setExcluded(first, true);
		session.toggleAll();
		session.toggle(first);
		session.toggle(second);
		session.setExclude(false);
		expect(session.allExcluded).toBe(false);
		expect(session.ruleChanges).toEqual([]);
	});
});
