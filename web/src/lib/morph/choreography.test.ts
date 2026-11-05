import { describe, expect, it } from 'vitest';
import { Channel, landsWithin } from './channel';
import {
	blend,
	bloomStrength,
	deckToHome,
	drop,
	choreographyFor,
	linearEasing,
	parseTime,
	resolveMotion,
	rippleDelay,
	rippleWeight,
	settleTime,
	springParams,
	stepSpring,
	weighPart,
	type Motion
} from './choreography';

const motion: Motion = { duration: 600, bounce: 0 };

function curve(easing: string): number[] {
	return easing
		.slice('linear('.length, -1)
		.split(', ')
		.map((point) => Number(point));
}

describe('springParams', () => {
	it('derives a critically damped spring from no bounce', () => {
		const { stiffness, damping } = springParams({ duration: 500, bounce: 0 });
		expect(damping).toBeCloseTo(2 * Math.sqrt(stiffness));
	});

	it('lowers damping with bounce and shortens the spring with duration', () => {
		const soft = springParams({ duration: 500, bounce: 0.2 });
		const firm = springParams({ duration: 500, bounce: 0 });
		const quick = springParams({ duration: 250, bounce: 0 });
		expect(soft.damping).toBeLessThan(firm.damping);
		expect(quick.stiffness).toBeCloseTo(firm.stiffness * 4);
	});
});

describe('stepSpring', () => {
	for (const bounce of [0, 0.08, 0.3]) {
		it(`gives the same answer in one step or many (bounce ${String(bounce)})`, () => {
			const params = springParams({ duration: 400, bounce });
			const start = { offset: 120, velocity: -300 };
			const once = stepSpring(params, start, 0.2);
			let many = start;
			for (let frame = 0; frame < 20; frame += 1) {
				many = stepSpring(params, many, 0.01);
			}
			expect(many.offset).toBeCloseTo(once.offset, 6);
			expect(many.velocity).toBeCloseTo(once.velocity, 6);
		});

		it(`obeys the spring equation (bounce ${String(bounce)})`, () => {
			const params = springParams({ duration: 400, bounce });
			const here = stepSpring(params, { offset: 1, velocity: 0 }, 0.1);
			const dt = 1e-5;
			const next = stepSpring(params, here, dt);
			const acceleration = (next.velocity - here.velocity) / dt;
			expect(acceleration).toBeCloseTo(
				-params.stiffness * here.offset - params.damping * here.velocity,
				1
			);
		});
	}

	it('comes to rest', () => {
		const state = stepSpring(springParams(motion), { offset: 1, velocity: 0 }, 5);
		expect(Math.abs(state.offset)).toBeLessThan(1e-6);
	});
});

describe('settleTime', () => {
	it('is near the perceived duration', () => {
		const settle = settleTime(motion);
		expect(settle).toBeGreaterThan(motion.duration * 0.9);
		expect(settle).toBeLessThan(motion.duration * 1.6);
	});
});

describe('linearEasing', () => {
	it('starts at 0 and ends at 1', () => {
		const points = curve(linearEasing(motion));
		expect(points.at(0)).toBe(0);
		expect(points.at(-1)).toBe(1);
	});

	it('never overshoots without bounce', () => {
		const points = curve(linearEasing({ duration: 420, bounce: 0 }));
		expect(Math.max(...points)).toBeLessThanOrEqual(1);
		expect(points).toEqual([...points].sort((a, b) => a - b));
	});

	it('stays restrained even with some bounce', () => {
		expect(Math.max(...curve(linearEasing({ duration: 600, bounce: 0.08 })))).toBeLessThan(1.002);
	});

	it('is the same curve the spring runs', () => {
		const samples = 48;
		const points = curve(linearEasing(motion, samples));
		const interval = settleTime(motion) / 1000 / samples;
		const halfway = stepSpring(
			springParams(motion),
			{ offset: 1, velocity: 0 },
			interval * (samples / 2)
		);
		expect(points.at(samples / 2)).toBeCloseTo(1 - halfway.offset, 3);
	});
});

describe('Channel', () => {
	it('keeps its position and speed when the target changes mid-flight', () => {
		const params = springParams(motion);
		const channel = new Channel(0, 500);
		channel.step(params, 0.15);
		const { value, velocity } = channel;
		expect(value).toBeGreaterThan(0);
		channel.target = -200;
		expect(channel.value).toBe(value);
		expect(channel.velocity).toBe(velocity);

		// The first frame after the turn moves on from here, by no more than its speed allows.
		const before = channel.value;
		channel.step(params, 1 / 60);
		expect(Math.abs(channel.value - before)).toBeLessThan((Math.abs(velocity) / 60) * 1.5);
	});

	it('reaches a moving target', () => {
		const params = springParams(motion);
		const channel = new Channel(0, 100);
		for (let frame = 0; frame < 180; frame += 1) {
			channel.target = 100 + frame * 0.1;
			channel.step(params, 1 / 60);
		}
		expect(channel.value).toBeGreaterThan(110);
	});

	it('takes over another channel exactly', () => {
		const source = new Channel(0, 1);
		source.step(springParams(motion), 0.1);
		const follower = new Channel(5, 0);
		follower.adopt(source);
		expect(follower.value).toBe(source.value);
		expect(follower.velocity).toBe(source.velocity);
	});
});

describe('bloom', () => {
	it('is dark at both ends and reaches its peak between them', () => {
		expect(bloomStrength(0, 0.9)).toBe(0);
		expect(bloomStrength(1, 0.9)).toBe(0);
		expect(bloomStrength(0.4, 0.9)).toBeCloseTo(0.9, 2);
	});

	it('tolerates a spring overshooting either end', () => {
		expect(bloomStrength(1.01, 0.9)).toBe(0);
		expect(bloomStrength(-0.01, 0.9)).toBe(0);
	});
});

describe('ripple', () => {
	const ripple = { spread: 40, floor: 0.08 };
	const leaving = choreographyFor('home', 'album').leaving;

	it('moves the units beside the cover most, and the far ones barely', () => {
		expect(rippleWeight(ripple, 0)).toBe(1);
		expect(rippleWeight(ripple, 0.5)).toBeLessThan(0.4);
		expect(rippleWeight(ripple, 1)).toBeCloseTo(0.08);
	});

	it('is never uniform: delay, distance and scale all vary across the page', () => {
		const near = weighPart(leaving, rippleWeight(ripple, 0.1));
		const far = weighPart(leaving, rippleWeight(ripple, 0.9));
		expect(near.to.scale).toBeLessThan(far.to.scale ?? 1);
		expect(rippleDelay(ripple, 0.9)).toBeGreaterThan(rippleDelay(ripple, 0.1));
	});

	it('leaves opacity alone, so far units still fade', () => {
		expect(weighPart(leaving, 0.1).to.opacity).toBe(0);
	});

	it('finishes within about 300 ms, far edge included', () => {
		const exit = resolveMotion(leaving.motion, () => 280);
		expect(settleTime(exit) + (leaving.ripple?.spread ?? 0)).toBeLessThanOrEqual(320);
	});

	it('adds no spread on the way back: an entrance must not queue', () => {
		expect(choreographyFor('album', 'home').arriving.card?.ripple?.spread).toBe(0);
	});
});

describe('landing', () => {
	function framesToLand(tolerance: number): number {
		const params = springParams(motion);
		const channel = new Channel(0, 900);
		let frames = 0;
		while (!channel.settled(tolerance) && frames < 600) {
			channel.step(params, 1 / 60);
			frames += 1;
		}
		return frames;
	}

	it('stops at about a pixel, well before the spring has finished creeping', () => {
		expect(landsWithin).toBe(1);
		expect(framesToLand(0.4)).toBeGreaterThan(framesToLand(landsWithin));
	});

	it('lands within a pixel of the target', () => {
		const params = springParams(motion);
		const channel = new Channel(0, 900);
		while (!channel.settled(landsWithin)) {
			channel.step(params, 1 / 60);
		}
		expect(Math.abs(channel.value - 900)).toBeLessThan(landsWithin);
	});
});

describe('choreographyFor', () => {
	const routes = ['home', 'album', 'now-playing'] as const;
	const depth = { home: 0, album: 1, 'now-playing': 2 };
	const pairs = routes.flatMap((from) =>
		routes.filter((to) => to !== from).map((to) => [from, to] as const)
	);

	it('has a journey for every pair of pages', () => {
		for (const [from, to] of pairs) {
			expect(choreographyFor(from, to)).not.toBe(blend);
		}
		expect(new Set(pairs.map(([from, to]) => choreographyFor(from, to).name)).size).toBe(6);
	});

	it('goes deeper one way and comes back the other, on every journey', () => {
		for (const [from, to] of pairs) {
			const { leaving } = choreographyFor(from, to);
			if (depth[to] > depth[from]) {
				expect(leaving.to.scale).toBeLessThan(1);
				expect(leaving.ripple).toBeDefined();
			} else {
				expect(leaving.to.y).toBeGreaterThan(0);
				expect(leaving.ripple).toBeUndefined();
			}
		}
	});

	it('takes less time coming back than going in', () => {
		for (const [from, to] of pairs) {
			if (depth[to] > depth[from]) {
				expect(choreographyFor(from, to).motion.token).toBe('--morph-slow');
				expect(choreographyFor(to, from).motion.token).toBe('--morph');
			}
		}
	});

	it('reshapes the island exactly when Now playing is involved', () => {
		for (const [from, to] of pairs) {
			expect(choreographyFor(from, to).island !== null).toBe(
				from === 'now-playing' || to === 'now-playing'
			);
		}
	});

	it('blends a page into itself', () => {
		expect(choreographyFor('home', 'home')).toBe(blend);
	});

	it('blends to and from a page with no journey yet', () => {
		for (const page of ['search', 'artists', 'artist'] as const) {
			for (const other of [...routes, 'login', 'search', 'artists', 'artist'] as const) {
				expect(choreographyFor(other, page)).toBe(blend);
				expect(choreographyFor(page, other)).toBe(blend);
			}
		}
	});
});

describe('the door', () => {
	const away = ['home', 'album', 'now-playing'] as const;

	it('sends the fox to the nav, going deeper, into Home and Album', () => {
		for (const to of ['home', 'album'] as const) {
			const { travel, leaving, motion, islandForms } = choreographyFor('login', to);
			expect(travel).toMatchObject({ subject: 'fox', sourceSlots: ['door'], targetSlots: ['nav'] });
			expect(leaving.to.scale).toBeLessThan(1);
			expect(motion.token).toBe('--morph-slow');
			expect(islandForms?.from.opacity).toBe(0);
		}
	});

	it('leaves the island alone apart from forming it: nothing reshapes', () => {
		expect(choreographyFor('login', 'home').island).toBeNull();
		expect(choreographyFor('login', 'album').island).toBeNull();
	});

	it('moves every unit it brings in, with no spread: an entrance must not queue', () => {
		const { arriving } = choreographyFor('login', 'home');
		expect(arriving.intro?.delay).toBe(arriving.card?.delay);
	});

	it('blends into Now playing, and back to the door from anywhere', () => {
		expect(choreographyFor('login', 'now-playing')).toBe(blend);
		for (const from of away) {
			expect(choreographyFor(from, 'login')).toBe(blend);
		}
	});
});

describe('parseTime', () => {
	it('reads both CSS time units', () => {
		expect(parseTime(' 420ms')).toBe(420);
		expect(parseTime('0.6s')).toBe(600);
	});
});

describe('the deal', () => {
	it('leaves the greeting still, and lands the fliers on what is already in place', () => {
		expect(deckToHome.arriving.intro).toBeUndefined();
		expect(deckToHome.arrivingByKey?.['feature']?.from.y).toBeUndefined();
		expect(deckToHome.arrivingByKey?.['recent']?.from.y).toBeUndefined();
	});

	it('rises the rest of Home as cards rise through the door', () => {
		expect(deckToHome.arriving.card).toEqual(choreographyFor('login', 'home').arriving.card);
	});

	it('is only scale and opacity on the way down, and never overshoots', () => {
		expect(deckToHome.leaving.to.y).toBeUndefined();
		expect(deckToHome.leaving.motion.bounce).toBe(0);
		expect(drop(5).motion.bounce).toBe(0);
	});

	it('throws a card from off the pile, and settles it into its own tilt', () => {
		const { from, to } = drop(5);
		expect(from.y).toBeLessThan(-28);
		expect(from.scale).toBeGreaterThan(1.08);
		expect(from.rotate).toBe(5);
		expect(to).toEqual({});
	});

	it('is solid well before it lands, so cards landing together never dissolve into each other', () => {
		const { fade, motion } = drop(5);
		const length = (spec: typeof motion): number => resolveMotion(spec, () => 180).duration;
		expect(length(fade ?? motion)).toBeLessThan(length(motion) / 2);
	});
});
