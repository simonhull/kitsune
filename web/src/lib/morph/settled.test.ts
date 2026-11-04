import { afterEach, describe, expect, it, vi } from 'vitest';
import { markMoving, whenSettled } from './settled';

afterEach(() => {
	markMoving(false);
});

describe('whenSettled', () => {
	it('runs at once when nothing is in flight', () => {
		const work = vi.fn();
		whenSettled(work);
		expect(work).toHaveBeenCalledOnce();
	});

	it('waits for the landing, and runs once', () => {
		const work = vi.fn();
		markMoving(true);
		whenSettled(work);
		expect(work).not.toHaveBeenCalled();
		markMoving(false);
		markMoving(false);
		expect(work).toHaveBeenCalledOnce();
	});

	it('can be called off, as when the page has gone', () => {
		const work = vi.fn();
		markMoving(true);
		whenSettled(work)();
		markMoving(false);
		expect(work).not.toHaveBeenCalled();
	});
});
