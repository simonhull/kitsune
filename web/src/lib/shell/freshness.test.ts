import { describe, expect, it } from 'vitest';
import { reloadsInto, type Freshness } from './freshness.ts';

const waiting: Freshness = { updated: true, sounding: false, started: false, type: 'link' };

describe('reloadsInto', () => {
	it('reloads on a link when a build waits and nothing has started', () => {
		expect(reloadsInto(waiting)).toBe(true);
	});
	it('never reloads while a song sounds', () => {
		expect(reloadsInto({ ...waiting, sounding: true })).toBe(false);
	});
	it('never reloads while a queue has started, even paused', () => {
		expect(reloadsInto({ ...waiting, started: true })).toBe(false);
	});
	it('does not reload when no build waits', () => {
		expect(reloadsInto({ ...waiting, updated: false })).toBe(false);
	});
	it('does not reload on a leave or an enter', () => {
		expect(reloadsInto({ ...waiting, type: 'leave' })).toBe(false);
		expect(reloadsInto({ ...waiting, type: 'enter' })).toBe(false);
	});
});
