import { describe, expect, it } from 'vitest';
import { Channel } from './channel';
import {
	blend,
	choreographyFor,
	linearEasing,
	parseTime,
	settleTime,
	springParams,
	stepSpring,
	type Motion
} from './choreography';

const motion: Motion = { duration: 600, bounce: 0.08 };

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
	it('is near the perceived duration, and longer than it', () => {
		const settle = settleTime(motion);
		expect(settle).toBeGreaterThan(motion.duration);
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

	it('stays restrained: nothing reads as bouncy', () => {
		expect(Math.max(...curve(linearEasing(motion)))).toBeLessThan(1.002);
	});

	it('is the same curve the spring engine runs', () => {
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

describe('choreographyFor', () => {
	it('has a journey for each direction', () => {
		expect(choreographyFor('home', 'album').name).toBe('home-to-album');
		expect(choreographyFor('album', 'home').name).toBe('album-to-home');
	});

	it('blends what has no journey yet', () => {
		expect(choreographyFor('home', 'now-playing')).toBe(blend);
	});
});

describe('parseTime', () => {
	it('reads both CSS time units', () => {
		expect(parseTime(' 420ms')).toBe(420);
		expect(parseTime('0.6s')).toBe(600);
	});
});
