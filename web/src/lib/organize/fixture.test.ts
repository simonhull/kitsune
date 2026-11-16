import { describe, expect, it, vi } from 'vitest';
import { createOrganizeFixture } from './fixture.ts';
import { defaultCompilations, defaultTemplate } from './template.ts';
import type { OrganizeEvent } from './types.ts';

function fixture(writes = true) {
	return createOrganizeFixture({
		albums: () => [],
		templates: () => ({ template: defaultTemplate, compilations: defaultCompilations }),
		writes: () => writes,
		albumMs: 10,
		respond: (answer) => Promise.resolve().then(answer)
	});
}

describe('the organize fixture', () => {
	it('previews every kind of song the boards show', async () => {
		const source = fixture();
		const kept = await source.preview({});
		expect(kept.counts).toMatchObject({ conflicts: 3, skipped: 1 });
		const listed = await source.page(kept.id, { filter: 'conflict' });
		const files = listed.albums.flatMap((a) => a.files);
		expect(files.some((f) => f.reason === 'unknownFile')).toBe(true);
		expect(files.some((f) => f.conflict?.sameRecording === true)).toBe(true);
		const arcadia = (await source.page(kept.id, { filter: 'move' })).albums.find(
			(a) => a.title === 'Even in Arcadia'
		);
		expect(arcadia?.caseOnly).toBe(true);
	});

	it('runs album by album, telling as it goes, and undoes the run', async () => {
		vi.useFakeTimers();
		const source = fixture();
		const heard: OrganizeEvent[] = [];
		source.listen?.((event) => heard.push(event));
		const kept = await source.preview({});
		const run = await source.start({ previewId: kept.id });
		await vi.runAllTimersAsync();
		expect(heard.at(-1)).toMatchObject({ run: run.id, state: 'done', done: kept.counts.moves });
		const again = await source.preview({});
		expect(again.counts.moves).toBe(0);
		const undo = await source.undo(run.id);
		await vi.runAllTimersAsync();
		expect(heard.at(-1)).toMatchObject({ run: undo.id, state: 'done' });
		expect((await source.preview({})).counts.moves).toBe(kept.counts.moves);
		expect((await source.run(run.id)).run.state).toBe('undone');
		vi.useRealTimers();
	});

	it('refuses a run with tag writes off', async () => {
		const source = fixture(false);
		const kept = await source.preview({});
		await expect(source.start({ previewId: kept.id })).rejects.toMatchObject({ status: 409 });
	});
});
