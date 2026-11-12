import { describe, expect, it } from 'vitest';
import { reloadsInto, type Freshness } from './freshness.ts';

const waiting: Freshness = { updated: true, sounding: false, unsaved: false, type: 'link' };

describe('reloadsInto', () => {
	it('reloads on a link when a build waits and nothing would be lost', () => {
		expect(reloadsInto(waiting)).toBe(true);
	});
	it('never reloads while a song sounds, saved or not', () => {
		expect(reloadsInto({ ...waiting, sounding: true })).toBe(false);
		expect(reloadsInto({ ...waiting, sounding: true, unsaved: true })).toBe(false);
	});
	it('never reloads a paused queue the server does not hold yet', () => {
		expect(reloadsInto({ ...waiting, unsaved: true })).toBe(false);
	});
	it('does not reload when no build waits', () => {
		expect(reloadsInto({ ...waiting, updated: false })).toBe(false);
	});
	it('does not reload on a leave or an enter', () => {
		expect(reloadsInto({ ...waiting, type: 'leave' })).toBe(false);
		expect(reloadsInto({ ...waiting, type: 'enter' })).toBe(false);
	});
});
