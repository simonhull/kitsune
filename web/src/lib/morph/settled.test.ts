import { afterEach, describe, expect, it, vi } from 'vitest';
import { holdSettled, markMoving, whenSettled } from './settled';

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

	it('waits for a hold, through any number of morphs starting and landing under it', () => {
		const work = vi.fn();
		const release = holdSettled();
		whenSettled(work);
		markMoving(true);
		markMoving(false);
		expect(work).not.toHaveBeenCalled();
		release();
		expect(work).toHaveBeenCalledOnce();
	});

	it('lets go of a hold only once, and never under a morph still moving', () => {
		const work = vi.fn();
		const first = holdSettled();
		const second = holdSettled();
		whenSettled(work);
		first();
		first();
		expect(work).not.toHaveBeenCalled();
		markMoving(true);
		second();
		expect(work).not.toHaveBeenCalled();
		markMoving(false);
		expect(work).toHaveBeenCalledOnce();
	});
});
