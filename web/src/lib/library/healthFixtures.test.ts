import { describe, expect, it } from 'vitest';
import { createFixtureSource } from './fixtures';
import { createHealthFixture, healthFixture } from './healthFixtures';
import { ServerError } from './source';

function clock(start = 1_000_000_000_000) {
	let now = start;
	return {
		read: () => now,
		advance(ms: number) {
			now += ms;
		}
	};
}

describe('healthFixture', () => {
	it('reads the URL, and opens on the board for anything else', () => {
		expect(healthFixture('running')).toBe('running');
		expect(healthFixture('nonsense')).toBe('idle');
		expect(healthFixture(null)).toBe('idle');
	});
});

describe('the Health fixtures', () => {
	it('opens idle on the board: 16 WAV files, 1 warning, a last scan', () => {
		const { issues, issueCounts, scan } = createHealthFixture('idle', 'admin').facts();
		expect(issues).toBe(16);
		expect(issueCounts).toEqual({ unreadable: 0, corrupted: 0, unsupported: 16, warnings: 1 });
		expect(scan.state).toBe('idle');
		expect(scan.last?.outcome).toBe('finished');
	});

	it('opens running on the board: 3,120 of 7,602', () => {
		const time = clock();
		expect(createHealthFixture('running', 'admin', time.read).facts().scan).toMatchObject({
			state: 'running',
			mode: 'full',
			read: 3120,
			total: 7602
		});
	});

	it('has no total while starting, then gets one', () => {
		const time = clock();
		const scans = createHealthFixture('starting', 'admin', time.read);
		expect(scans.facts().scan).toMatchObject({ state: 'running', read: 0, total: 0 });
		time.advance(3000);
		expect(scans.facts().scan.total).toBe(7602);
	});

	it('has nothing to say when clean', () => {
		const scans = createHealthFixture('clean', 'admin');
		expect(scans.facts().issues).toBe(0);
		expect(scans.issues()).toEqual({ issues: [], warnings: [] });
	});

	it('counts what it lists', () => {
		const scans = createHealthFixture('mixed', 'admin');
		const { issues, issueCounts } = scans.facts();
		expect(scans.issues().issues).toHaveLength(issues);
		expect(scans.issues().warnings).toHaveLength(issueCounts.warnings);
		expect(issueCounts.corrupted).toBeGreaterThan(0);
		expect(issueCounts.unreadable).toBeGreaterThan(0);
	});

	it('gives the reason for a failure to an admin alone', () => {
		const admin = createHealthFixture('failed', 'admin').facts().scan.last;
		expect(admin?.outcome).toBe('failed');
		expect(admin?.error).toBeDefined();
		const user = createHealthFixture('failed', 'user').facts().scan.last;
		expect(user?.outcome).toBe('failed');
		expect(user).not.toHaveProperty('error');
	});

	it('refuses a non-admin the paths and the buttons, as the server does', () => {
		const scans = createHealthFixture('idle', 'user');
		expect(() => scans.issues()).toThrow(ServerError);
		expect(() => scans.start('quick')).toThrow(ServerError);
		expect(scans.facts().issueCounts.unsupported).toBe(16);
	});

	it('runs a scan from the buttons to its end, and refuses a second', () => {
		const time = clock();
		const scans = createHealthFixture('clean', 'admin', time.read);
		expect(scans.start('quick').state).toBe('running');
		expect(() => scans.start('full')).toThrow(ServerError);
		time.advance(3000);
		expect(scans.facts().scan).toMatchObject({ state: 'running', mode: 'quick', total: 12 });
		time.advance(2000);
		expect(scans.facts().scan).toMatchObject({ state: 'idle', last: { mode: 'quick' } });
		expect(scans.start('full').state).toBe('running');
	});

	it('reaches the page through the source, role and all', async () => {
		const source = createFixtureSource({ health: 'running', role: 'user' });
		await expect(source.me()).resolves.toMatchObject({ role: 'user' });
		await expect(source.facts()).resolves.toMatchObject({ scan: { state: 'running' } });
		await expect(source.startScan('full')).rejects.toBeInstanceOf(ServerError);
	});
});
