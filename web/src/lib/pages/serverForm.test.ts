import { describe, expect, it } from 'vitest';
import { settingsFixture } from '#lib/library/settingsFixtures.ts';
import {
	afterSave,
	changeFor,
	daysText,
	dirty,
	draftOf,
	durationDays,
	folderLine,
	fromLine,
	graceOf,
	lastScanLine,
	proxiesOf,
	restartNote,
	size,
	undone
} from './serverForm.ts';

const server = settingsFixture(true);

describe('the Server page form', () => {
	it('starts clean, as the server has it', () => {
		const base = draftOf(server);
		expect(base).toMatchObject({
			roots: ['/music'],
			quickScan: '03:00',
			graceDays: '30',
			level: 'info'
		});
		for (const section of ['library', 'lastfm', 'scanning', 'advanced'] as const) {
			expect(dirty(base, draftOf(server), section)).toBe(false);
		}
	});

	it('marks only the section that changed', () => {
		const base = draftOf(server);
		const draft = { ...draftOf(server), apiKey: 'new' };
		expect(dirty(base, draft, 'lastfm')).toBe(true);
		expect(dirty(base, draft, 'library')).toBe(false);
	});

	it('sends only the keys that changed', () => {
		const base = draftOf(server);
		const draft = { ...draftOf(server), roots: ['/music', '/audiobooks'], graceDays: '7' };
		expect(changeFor(base, draft, 'library')).toEqual({
			change: { 'library.roots': ['/music', '/audiobooks'] }
		});
		expect(changeFor(base, draft, 'scanning')).toEqual({
			change: { 'library.missing_grace': '604800s' }
		});
		expect(changeFor(base, base, 'advanced')).toEqual({ change: {} });
	});

	it('writes the secret only when it is replaced or cleared', () => {
		const base = draftOf(server);
		expect(changeFor(base, base, 'lastfm')).toEqual({ change: {} });
		expect(
			changeFor(base, { ...base, secret: { kind: 'replace', text: ' s3 ' } }, 'lastfm')
		).toEqual({
			change: { 'lastfm.shared_secret': 's3' }
		});
		expect(changeFor(base, { ...base, secret: { kind: 'clear' } }, 'lastfm')).toEqual({
			change: { 'lastfm.shared_secret': null }
		});
		expect(
			changeFor(base, { ...base, secret: { kind: 'replace', text: '' } }, 'lastfm')
		).toMatchObject({
			problem: { key: 'lastfm.shared_secret' }
		});
	});

	it('refuses what the server would, before asking it', () => {
		const base = draftOf(server);
		expect(changeFor(base, { ...base, graceDays: 'soon' }, 'scanning')).toMatchObject({
			problem: { key: 'library.missing_grace' }
		});
		expect(changeFor(base, { ...base, graceDays: '-1' }, 'scanning')).toMatchObject({
			problem: {}
		});
		expect(changeFor(base, { ...base, quickScan: '' }, 'scanning')).toMatchObject({
			problem: { key: 'library.quick_scan' }
		});
	});

	it('splits the proxies on commas', () => {
		const base = draftOf(server);
		expect(changeFor(base, { ...base, proxies: '10.0.0.2, ,172.17.0.0/16 ' }, 'advanced')).toEqual({
			change: { 'server.trusted_proxies': ['10.0.0.2', '172.17.0.0/16'] }
		});
		expect(proxiesOf('')).toEqual([]);
	});

	it('undoes one section and leaves the others', () => {
		const base = draftOf(server);
		const draft = { ...base, apiKey: 'typed', level: 'debug' as const };
		const after = undone(base, draft, 'lastfm');
		expect(after.apiKey).toBe(base.apiKey);
		expect(after.level).toBe('debug');
	});

	it('keeps what is being typed elsewhere through a save', () => {
		const oldBase = draftOf(server);
		const draft = { ...oldBase, apiKey: 'typed', quickScan: '05:00' };
		const newBase = { ...oldBase, quickScan: '05:00', tagWrites: true };
		const after = afterSave(oldBase, newBase, draft, 'scanning');
		expect(after).toMatchObject({ apiKey: 'typed', quickScan: '05:00', tagWrites: true });
	});
});

describe('the words on the Server page', () => {
	it('reads Go durations as days', () => {
		expect(durationDays('720h0m0s')).toBe(30);
		expect(durationDays('24h0m0s')).toBe(1);
		expect(durationDays('36h0m0s')).toBe(1.5);
		expect(durationDays('0s')).toBe(0);
		expect(durationDays('nonsense')).toBeNull();
		expect(daysText(1.5)).toBe('1.5');
		expect(graceOf('1.5')).toBe('129600s');
		expect(graceOf('')).toBeNull();
	});

	it('says where a value came from only when it was a file, a variable or a flag', () => {
		expect(fromLine({ source: 'env', from: 'KITSUNE_LASTFM_SHARED_SECRET' })).toBe(
			'FIRST SET FROM KITSUNE_LASTFM_SHARED_SECRET'
		);
		expect(fromLine({ source: 'server.toml', from: 'server.toml' })).toBe(
			'FIRST SET FROM server.toml'
		);
		expect(fromLine({ source: 'web' })).toBeNull();
		expect(fromLine({ source: 'default' })).toBeNull();
	});

	it('marks the settings that wait for a restart', () => {
		expect(restartNote({ value: '', source: 'default', restartRequired: true })).toBe(
			'AFTER A RESTART'
		);
		expect(
			restartNote({ value: 'x', source: 'web', restartRequired: true, pendingRestart: true })
		).toBe('SAVED · AFTER A RESTART');
		expect(restartNote({ value: 'info', source: 'default' })).toBeNull();
	});

	it('sizes a library', () => {
		expect(size(140_660_000_000)).toBe('141 GB');
		expect(size(1_250_000_000)).toBe('1.3 GB');
		expect(size(0)).toBe('0 B');
		expect(folderLine(4350, 131_000_000_000, true)).toBe('4,350 songs · 131 GB');
		expect(folderLine(0, 0, true)).toBe('Nothing read yet');
		expect(folderLine(0, 0, false)).toMatch(/when you save/);
	});

	it('says when the last scan was', () => {
		const now = new Date(2026, 10, 7, 12, 0).getTime();
		const last = {
			mode: 'quick' as const,
			outcome: 'finished' as const,
			startedAt: new Date(2026, 10, 7, 3, 0).toISOString(),
			endedAt: new Date(2026, 10, 7, 3, 4).toISOString(),
			seen: 1,
			read: 1,
			added: 0,
			updated: 0,
			moved: 0,
			missing: 0,
			deleted: 0,
			issues: 0,
			skippedRoots: 0
		};
		expect(lastScanLine(last, false, now)).toBe('Today 03:04 · took 4 min');
		expect(lastScanLine(undefined, false, now)).toBe('None since the server started');
		expect(lastScanLine(last, true, now)).toBe('Running now');
	});
});
