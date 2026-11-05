import { afterEach, describe, expect, it, vi } from 'vitest';
import { markMoving } from '#lib/morph/settled.ts';
import { createCarrier, type CarryParts } from './carry';

function setup(parts: Partial<CarryParts> = {}) {
	const blend = vi.fn((change: () => Promise<void>) => change());
	const carrier = createCarrier({
		hidden: () => false,
		onNowPlaying: () => false,
		blend,
		...parts
	});
	return { carrier, blend };
}

describe('createCarrier', () => {
	afterEach(() => {
		markMoving(false);
	});

	it('swaps at once with nothing in the air', () => {
		const { carrier, blend } = setup();
		const apply = vi.fn();
		carrier.carry(apply);
		expect(apply).toHaveBeenCalledOnce();
		expect(blend).not.toHaveBeenCalled();
	});

	it('waits for a flight', () => {
		const { carrier } = setup();
		const apply = vi.fn();
		markMoving(true);
		carrier.carry(apply);
		expect(apply).not.toHaveBeenCalled();
		markMoving(false);
		expect(apply).toHaveBeenCalledOnce();
	});

	it('blends on Now playing, after whatever is in the air', () => {
		const { carrier, blend } = setup({ onNowPlaying: () => true });
		const apply = vi.fn();
		markMoving(true);
		carrier.carry(apply);
		expect(blend).not.toHaveBeenCalled();
		markMoving(false);
		expect(blend).toHaveBeenCalledOnce();
		expect(apply).toHaveBeenCalledOnce();
	});

	it('sets the change synchronously inside the blend, before the picture is taken', () => {
		const order: string[] = [];
		const { carrier } = setup({
			onNowPlaying: () => true,
			blend: (change) => {
				const changed = change();
				order.push('picture');
				return changed;
			}
		});
		carrier.carry(() => order.push('state'));
		expect(order).toEqual(['state', 'picture']);
	});

	it('applies at once in a hidden tab, with no blend and no hold', () => {
		const { carrier, blend } = setup({ hidden: () => true, onNowPlaying: () => true });
		const apply = vi.fn();
		markMoving(true);
		carrier.carry(apply);
		expect(apply).toHaveBeenCalledOnce();
		expect(blend).not.toHaveBeenCalled();
	});

	it('lets a later change replace one still waiting', () => {
		const { carrier } = setup();
		const [first, second] = [vi.fn(), vi.fn()];
		markMoving(true);
		carrier.carry(first);
		carrier.carry(second);
		markMoving(false);
		expect(first).not.toHaveBeenCalled();
		expect(second).toHaveBeenCalledOnce();
	});

	it('hands over what waits when the tab goes hidden, once', () => {
		const { carrier } = setup();
		const apply = vi.fn();
		markMoving(true);
		carrier.carry(apply);
		carrier.flush();
		expect(apply).toHaveBeenCalledOnce();
		markMoving(false);
		expect(apply).toHaveBeenCalledOnce();
	});

	it('has nothing to hand over otherwise', () => {
		const { carrier } = setup();
		expect(() => {
			carrier.flush();
		}).not.toThrow();
	});
});
