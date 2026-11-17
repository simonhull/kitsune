import { describe, expect, it } from 'vitest';
import { Channel, landsWithin } from './channel';
import {
	blend,
	bloomStrength,
	deckToHome,
	drop,
	choreographyFor,
	gatherWeight,
	weighGather,
	linearEasing,
	parseTime,
	resolveMotion,
	rippleDelay,
	rippleWeight,
	settleTime,
	springParams,
	stepSpring,
	weighPart,
	type Grain,
	type Motion,
	type RouteName
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

	it('adds no spread to a gathering: an entrance must not queue', () => {
		for (const part of Object.values(choreographyFor('album', 'home').arriving)) {
			expect(part.ripple).toBeUndefined();
			expect(part.delay).toBe(90);
		}
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

/**
 * The grain of the library, which decides every journey: the door, then the rooms, then the
 * records, then the moment. A record that holds another (an artist or a playlist holds albums) is a
 * step outside it. A move into what a page holds opens, into what holds it gathers, and between two
 * of a kind crosses. Depth is how far you've come, so none of these is a way back.
 */
const level: Record<RouteName, number> = {
	login: 0,
	home: 1,
	library: 1,
	albums: 1,
	artists: 1,
	playlists: 1,
	search: 1,
	you: 1,
	health: 1,
	server: 1,
	artist: 2,
	playlist: 2,
	album: 3,
	'now-playing': 4
};

function grainOf(from: RouteName, to: RouteName): Grain {
	if (level[to] === level[from]) {
		return 'crossing';
	}
	return level[to] > level[from] ? 'opening' : 'gathering';
}

/** Every pair of milestone 1, and the shipped journeys it restacks. */
const journeyPairs: readonly (readonly [RouteName, RouteName])[] = [
	['home', 'album'],
	['library', 'album'],
	['albums', 'album'],
	['artist', 'album'],
	['playlist', 'album'],
	['search', 'album'],
	['album', 'home'],
	['album', 'library'],
	['album', 'albums'],
	['album', 'artist'],
	['artists', 'artist'],
	['library', 'artist'],
	['albums', 'artist'],
	['search', 'artist'],
	['artist', 'artists'],
	['artist', 'library'],
	['artist', 'albums'],
	['home', 'now-playing'],
	['album', 'now-playing'],
	['now-playing', 'home'],
	['now-playing', 'album'],
	['login', 'home'],
	['login', 'album']
];

describe('choreographyFor', () => {
	it('has a journey for every pair of the milestone', () => {
		for (const [from, to] of journeyPairs) {
			expect(choreographyFor(from, to), `${from} → ${to}`).not.toBe(blend);
		}
	});

	it('follows the grain on every journey: opening, gathering or crossing', () => {
		for (const [from, to] of journeyPairs) {
			expect(choreographyFor(from, to).grain, `${from} → ${to}`).toBe(grainOf(from, to));
		}
	});

	it('never moves the page you leave away to uncover the one you go to', () => {
		for (const [from, to] of journeyPairs) {
			const { leaving, leavingByRole = {}, held } = choreographyFor(from, to);
			for (const part of [leaving, ...Object.values(leavingByRole), ...Object.values(held)]) {
				// No sink: whatever leaves has no travel, only a fall back that something now covers.
				expect(part.to.y, `${from} → ${to}`).toBeUndefined();
				expect(part.to.opacity).toBe(0);
			}
		}
	});

	it('gives way where it stands on every gathering and crossing: no scale, no travel', () => {
		for (const [from, to] of journeyPairs) {
			const { grain, leaving, leavingByRole = {} } = choreographyFor(from, to);
			if (grain !== 'opening') {
				for (const part of [leaving, ...Object.values(leavingByRole)]) {
					expect(part.to, `${from} → ${to}`).toEqual({ opacity: 0 });
				}
			}
		}
	});

	it('plays both directions of a pair as their own journeys, never one backward', () => {
		for (const [from, to] of journeyPairs) {
			const back = choreographyFor(to, from);
			if (back !== blend && from !== to) {
				expect(back.name, `${from} ⇄ ${to}`).not.toBe(choreographyFor(from, to).name);
				expect(back.grain).not.toBe(choreographyFor(from, to).grain);
			}
		}
	});

	it('gathers quicker than it opens', () => {
		for (const [from, to] of journeyPairs) {
			const { grain, motion } = choreographyFor(from, to);
			if (grain === 'opening') {
				expect(motion.token, `${from} → ${to}`).toBe('--morph-slow');
			} else if (grain === 'gathering') {
				expect(motion.token, `${from} → ${to}`).toBe('--morph');
			}
		}
	});

	it('only ever lets light out of a cover: no gathering drains it back in', () => {
		for (const [from, to] of journeyPairs) {
			const { grain, bloom } = choreographyFor(from, to);
			if (grain !== 'opening') {
				expect(bloom, `${from} → ${to}`).toBeNull();
			}
		}
	});

	it('brings every entrance in at once, wherever it is on the page', () => {
		for (const [from, to] of journeyPairs) {
			for (const part of Object.values(choreographyFor(from, to).arriving)) {
				expect(part.ripple?.spread ?? 0, `${from} → ${to}`).toBe(0);
			}
		}
	});

	it('reshapes the album into the Continue card when it is Continue’s, and sets it among the others when not', () => {
		const into = choreographyFor('album', 'home', { continues: true });
		expect(into.name).toBe('album-into-continue');
		expect(into.travel?.leads).toContain('surface:from');
		expect(into.travel?.landIn).toBe('feature');

		const among = choreographyFor('album', 'home');
		expect(among.name).toBe('album-to-home');
		expect(among.travel?.landIn).toBe('played');
		expect(among.travel?.leads).not.toContain('surface:from');
	});

	it('grows the grid from the album: the Albums page gathers around the cell it lands in', () => {
		const { arriving } = choreographyFor('album', 'albums');
		expect(arriving.card?.gather).toBeDefined();
		expect(arriving.card?.from.scale).toBeLessThan(1);
	});

	it('lets the artist’s name take the stage a beat after the album’s headline has gone', () => {
		const { travel } = choreographyFor('album', 'artist');
		expect(travel?.leads).toEqual(['name:to', 'cover:from']);
		expect(travel?.waits?.name).toBe(40);
	});

	it('reshapes the island exactly when Now playing is involved', () => {
		for (const [from, to] of journeyPairs) {
			expect(choreographyFor(from, to).island !== null, `${from} → ${to}`).toBe(
				from === 'now-playing' || to === 'now-playing'
			);
		}
	});

	it('goes from the Library to an album as Home does, and comes back its own way', () => {
		const out = choreographyFor('library', 'album');
		const back = choreographyFor('album', 'library');
		expect(out.travel).toEqual(choreographyFor('home', 'album').travel);
		expect(back.name).not.toBe(choreographyFor('album', 'home').name);
		expect(back.bloom).toBeNull();
		// Nothing moves on its way out, so the album can't read as dropping away from a page behind it.
		expect(back.leaving.to).toEqual({ opacity: 0 });
	});

	it('blends a page into itself', () => {
		expect(choreographyFor('home', 'home')).toBe(blend);
	});

	it('blends every pair a later milestone takes: the rooms among themselves, and Search', () => {
		const rooms = ['home', 'library', 'albums', 'artists', 'playlists', 'search'] as const;
		for (const from of rooms) {
			for (const to of rooms) {
				expect(choreographyFor(from, to), `${from} → ${to}`).toBe(blend);
			}
		}
		for (const page of ['health', 'you', 'server'] as const) {
			expect(choreographyFor('album', page)).toBe(blend);
			expect(choreographyFor(page, 'album')).toBe(blend);
		}
		expect(choreographyFor('album', 'search')).toBe(blend);
		expect(choreographyFor('artist', 'artist')).toBe(blend);
	});
});

describe('gather', () => {
	const { arriving } = choreographyFor('album', 'albums');
	const part = arriving.card;

	it('is all but there beside the landing, and deepest at the far edge', () => {
		const near = { near: 0.15 };
		expect(gatherWeight(near, 0)).toBeCloseTo(0.15);
		expect(gatherWeight(near, 1)).toBe(1);
		expect(gatherWeight(near, 0.2)).toBeLessThan(gatherWeight(near, 0.8));
	});

	it('weighs only the depth: every unit fades and starts the same', () => {
		if (part === undefined) {
			throw new Error('Album → Albums has no card part');
		}
		const near = weighGather(part, 0.15);
		const far = weighGather(part, 1);
		expect(near.from.scale).toBeGreaterThan(far.from.scale ?? 1);
		expect(near.from.opacity).toBe(far.from.opacity);
		expect(near.delay).toBe(far.delay);
		expect(far.from.scale).toBe(part.from.scale);
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
