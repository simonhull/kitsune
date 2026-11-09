import { describe, expect, it } from 'vitest';
import {
	inkChanges,
	maxStepSeconds,
	overshootSlots,
	RailGoo,
	slotCenter,
	squashFloor,
	stretchSlots,
	type GooGeometry
} from './railGoo';

const open: GooGeometry = { step: 25, pill: 22 };
const folded: GooGeometry = { step: 10, pill: 18 };
const at = (index: number, geometry = open) => slotCenter(index, geometry.step);

/** A goo at rest on `index`, at a time well after any retarget. */
function restingOn(index: number, geometry = open): RailGoo {
	return new RailGoo(geometry, at(index, geometry));
}

/** Steps `goo` at `hz` for `seconds`, calling `each` after every frame. */
function run(goo: RailGoo, seconds: number, hz = 60, each?: () => void): void {
	const frames = Math.round(seconds * hz);
	for (let frame = 0; frame < frames; frame++) {
		goo.step(1 / hz);
		each?.();
	}
}

/** Seconds until both edges are within 1 px of their targets for good. */
function settleTime(goo: RailGoo, hz = 240): number {
	let last = 0;
	for (let frame = 1; frame <= hz * 2; frame++) {
		goo.step(1 / hz);
		const off = Math.max(
			Math.abs(goo.top.value - goo.top.target),
			Math.abs(goo.bottom.value - goo.bottom.target)
		);
		if (off >= 1) {
			last = frame / hz;
		}
	}
	return last;
}

/** A seeded generator, so the property test is the same every run. */
function random(seed: number): () => number {
	let state = seed;
	return () => {
		state = (state * 1_664_525 + 1_013_904_223) % 2 ** 32;
		return state / 2 ** 32;
	};
}

describe('RailGoo', () => {
	it('puts the leading edge at the new letter at once', () => {
		const goo = restingOn(2);
		goo.retarget(at(5), 1000);
		expect(goo.lead).toBe('bottom');
		expect(goo.bottom.value).toBe(at(5) + open.pill / 2);
		goo.retarget(at(0), 1500);
		expect(goo.lead).toBe('top');
		expect(goo.top.value).toBe(at(0) - open.pill / 2);
	});

	it('carries the trailing edge on across a retarget', () => {
		const goo = restingOn(4);
		goo.retarget(at(5), 1000);
		run(goo, 0.12);
		const { value, velocity } = goo.top;
		expect(velocity).not.toBe(0);
		// Onward, within the stretch: the tail is untouched by the retarget itself.
		goo.retarget(at(6), 1120);
		expect(goo.top.value).toBe(value);
		expect(goo.top.velocity).toBe(velocity);
		goo.step(1 / 240);
		expect(Math.abs(goo.top.value - value)).toBeLessThan(Math.abs(velocity) / 240 + 0.5);
	});

	it('settles within a pixel of the last letter, whatever came before', () => {
		const next = random(54);
		for (let trial = 0; trial < 200; trial++) {
			const goo = restingOn(Math.floor(next() * 27));
			goo.held = next() < 0.2;
			let now = 1000;
			let last = 0;
			const hops = 1 + Math.floor(next() * 8);
			for (let hop = 0; hop < hops; hop++) {
				last = Math.floor(next() * 27);
				goo.retarget(at(last), now);
				const gap = 0.02 + next() * 0.3;
				run(goo, gap);
				now += gap * 1000;
			}
			goo.held = false;
			run(goo, 0.6);
			expect(Math.abs(goo.center - at(last))).toBeLessThan(1);
			run(goo, 0.4);
			expect(goo.resting(), `trial ${String(trial)}`).toBe(true);
		}
	});

	it('keeps to the stretch cap, the squash floor and the overshoot cap', () => {
		for (const geometry of [open, folded]) {
			const goo = restingOn(0, geometry);
			const most = geometry.pill + stretchSlots * geometry.step;
			const least = geometry.pill * squashFloor;
			const over = geometry.step * overshootSlots + 1e-9;
			const check = () => {
				const length = goo.bottom.value - goo.top.value;
				expect(length).toBeLessThanOrEqual(most + 1e-9);
				expect(length).toBeGreaterThanOrEqual(least - 1e-9);
				for (const edge of [goo.top, goo.bottom]) {
					expect(Math.abs(edge.value - edge.target)).toBeLessThanOrEqual(
						// An edge behind its target isn't past it; only one beyond it is capped.
						Math.sign(edge.value - edge.target) === Math.sign(edge.velocity)
							? over
							: Number.POSITIVE_INFINITY
					);
				}
			};
			// Hop 20, back, then a sweep A to Z, a letter every 60 ms.
			goo.retarget(at(20, geometry), 1000);
			run(goo, 0.5, 60, check);
			goo.retarget(at(1, geometry), 2000);
			run(goo, 0.5, 60, check);
			for (let letter = 0; letter < 27; letter++) {
				goo.retarget(at(letter, geometry), 3000 + letter * 60);
				run(goo, 0.06, 60, check);
			}
			run(goo, 0.6, 60, check);
		}
	});

	it('takes as long to settle a hop of 20 as a hop of 1', () => {
		const one = restingOn(3);
		one.retarget(at(4), 1000);
		const twenty = restingOn(3);
		twenty.retarget(at(23), 1000);
		const short = settleTime(one);
		const long = settleTime(twenty);
		expect(short).toBeGreaterThan(0.1);
		expect(Math.abs(long - short) / short).toBeLessThan(0.2);
	});

	it('draws the same curve at 30, 60 and 144 Hz', () => {
		const sample = (hz: number) => {
			const goo = restingOn(3);
			goo.retarget(at(5), 1000);
			const curve: number[] = [];
			// Every 1/6 s, a frame the three rates share.
			for (let tick = 0; tick < 6; tick++) {
				run(goo, 1 / 6, hz);
				curve.push(goo.top.value, goo.bottom.value);
			}
			return curve;
		};
		const base = sample(144);
		for (const hz of [30, 60]) {
			sample(hz).forEach((value, i) => {
				expect(Math.abs(value - (base[i] ?? Number.NaN))).toBeLessThan(0.5);
			});
		}
	});

	it('changes nothing for a negative or zero step, and caps a long one', () => {
		const goo = restingOn(2);
		goo.retarget(at(6), 1000);
		run(goo, 0.03);
		const before = [goo.top.value, goo.top.velocity, goo.bottom.value, goo.bottom.velocity];
		goo.step(-0.003);
		goo.step(0);
		expect([goo.top.value, goo.top.velocity, goo.bottom.value, goo.bottom.velocity]).toEqual(
			before
		);
		const late = restingOn(2);
		late.retarget(at(6), 1000);
		const capped = restingOn(2);
		capped.retarget(at(6), 1000);
		late.step(1);
		capped.step(maxStepSeconds);
		expect(late.top.value).toBe(capped.top.value);
	});

	it('sheds a drop on a calm hop only', () => {
		const goo = restingOn(2);
		goo.retarget(at(5), 1000);
		const drop = goo.dropShape();
		expect(drop?.scale).toBe(1);
		expect(drop?.center).toBe(at(2));
		run(goo, 0.1);
		// It falls toward the new letter as it shrinks.
		expect(goo.dropShape()?.center).toBeGreaterThan(at(2));
		run(goo, 0.6);
		expect(goo.dropShape()).toBeUndefined();

		// Letters 80 ms apart are a sweep: no drops after the first.
		const busy = restingOn(2);
		busy.retarget(at(3), 1000);
		run(busy, 0.08);
		busy.retarget(at(4), 1080);
		expect(busy.busy).toBe(true);
		run(busy, 0.6);
		expect(busy.dropShape()).toBeUndefined();

		const held = restingOn(2);
		held.held = true;
		held.retarget(at(6), 1000);
		expect(held.dropShape()).toBeUndefined();
	});

	it('is busy for retargets under 120 ms apart, and calm again 400 ms on', () => {
		const goo = restingOn(2);
		goo.retarget(at(3), 1000);
		run(goo, 0.08);
		goo.retarget(at(4), 1080);
		expect(goo.busy).toBe(true);
		run(goo, 0.4);
		goo.retarget(at(5), 1480);
		expect(goo.busy).toBe(false);
		// The last letter of a sweep still gets its wobble once the sweep stops.
		const sweep = restingOn(2);
		sweep.retarget(at(3), 1000);
		run(sweep, 0.08);
		sweep.retarget(at(4), 1080);
		run(sweep, 0.15);
		expect(sweep.busy).toBe(false);
	});

	it('inks exactly the lit letter at rest, and what it covers on the way', () => {
		const goo = restingOn(4);
		expect(goo.inked()).toEqual({ from: 4, to: 4 });
		goo.retarget(at(6), 1000);
		goo.step(1 / 60);
		// Stretched from the old letter to the new one, it covers the letters between.
		const { from, to } = goo.inked();
		expect(from).toBeLessThanOrEqual(5);
		expect(to).toBe(6);
		run(goo, 0.8);
		expect(goo.inked()).toEqual({ from: 6, to: 6 });

		const small = restingOn(13, folded);
		expect(small.inked().from).toBeLessThanOrEqual(13);
		expect(small.inked().to).toBeGreaterThanOrEqual(13);
	});

	it('moves at once under reduced motion, leaving a ghost where it was', () => {
		const goo = restingOn(2);
		goo.reduced = true;
		const { ghost } = goo.retarget(at(9), 1000);
		expect(ghost).toBe(at(2));
		expect(goo.center).toBe(at(9));
		expect(goo.dropShape()).toBeUndefined();
		expect(goo.resting()).toBe(true);
		expect(goo.shape().lean).toBe(0);
		goo.squish();
		expect(goo.glyph.value).toBe(1);
	});

	it('squishes the bubble letter in and back to 1', () => {
		const goo = restingOn(2);
		goo.squish();
		expect(goo.glyph.value).toBeLessThan(1);
		expect(goo.resting()).toBe(false);
		run(goo, 0.8);
		expect(goo.glyph.value).toBeCloseTo(1, 2);
	});

	it('thins as it stretches and leans no more than 2 degrees', () => {
		const goo = restingOn(0);
		goo.retarget(at(20), 1000);
		goo.step(1 / 60);
		const shape = goo.shape();
		expect(shape.length).toBeGreaterThan(open.pill);
		expect(shape.width).toBeLessThan(1);
		expect(shape.width).toBeGreaterThanOrEqual(0.8);
		run(goo, 0.5, 60, () => {
			expect(Math.abs(goo.shape().lean)).toBeLessThanOrEqual(2);
		});
	});
});

describe('inkChanges', () => {
	const every = () => true;

	it('flips only the ends of a moving run', () => {
		expect(inkChanges({ from: 3, to: 4 }, { from: 4, to: 5 }, every)).toEqual([
			[3, false],
			[5, true]
		]);
		expect(inkChanges({ from: 3, to: 3 }, { from: 3, to: 3 }, every)).toEqual([]);
	});

	it('never inks a folded letter', () => {
		const drawn = (index: number) => index % 3 === 0;
		expect(inkChanges({ from: 0, to: -1 }, { from: 2, to: 6 }, drawn)).toEqual([
			[3, true],
			[6, true]
		]);
	});
});
