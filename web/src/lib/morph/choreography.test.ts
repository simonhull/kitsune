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
	rooms,
	unfold,
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
	trash: 1,
	organize: 1,
	duplicates: 1,
	matching: 2,
	duplicate: 2,
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

/** Every page the island shows on: each opens into the moment, and the moment goes back to each. */
const islandPages = [
	'home',
	'library',
	'albums',
	'artists',
	'playlists',
	'search',
	'you',
	'health',
	'server',
	'artist',
	'playlist',
	'album'
] as const satisfies readonly RouteName[];

/** Every pair of milestone 3: the player and Now playing, and the door opening on it. */
const momentPairs: readonly (readonly [RouteName, RouteName])[] = [
	...islandPages.map((page) => [page, 'now-playing'] as const),
	...islandPages.map((page) => ['now-playing', page] as const),
	['login', 'now-playing']
];

/** Every pair of milestones 1 and 3, and the shipped journeys they restack. */
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
	['album', 'playlist'],
	['album', 'album'],
	['artists', 'artist'],
	['library', 'artist'],
	['albums', 'artist'],
	['search', 'artist'],
	['artist', 'artists'],
	['artist', 'library'],
	['artist', 'albums'],
	['playlists', 'playlist'],
	['playlist', 'playlists'],
	['login', 'home'],
	['login', 'album'],
	...momentPairs
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

	it('holds the stage of an album it leaves, to give way under the page in front', () => {
		for (const to of ['home', 'library', 'albums', 'artist', 'playlist', 'album'] as const) {
			expect(choreographyFor('album', to).held.album?.to.opacity, to).toBe(0);
		}
		expect(choreographyFor('album', 'home', { continues: true }).held.album).toBeDefined();
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
		expect(travel?.leads).toEqual(['name:to', 'face:to', 'cover:from']);
		expect(travel?.waits?.name).toBe(40);
		expect(travel?.waits?.face).toBe(40);
	});

	it('brings the artist’s stage up in front where it stands as the album’s gives way', () => {
		const { stages, held } = choreographyFor('album', 'artist');
		expect(held.album?.to).toEqual({ opacity: 0 });
		expect(stages.artist?.from).toEqual({ opacity: 0 });
	});

	it('opens an artist as an album opens: the stage comes up, its picture never holding it', () => {
		for (const from of ['artists', 'library', 'search'] as const) {
			const { stages, travel } = choreographyFor(from, 'artist');
			expect(travel?.leads, from).toEqual(['name:to', 'face:to']);
			expect(stages.artist, from).toEqual(choreographyFor('albums', 'album').stages.album);
		}
	});

	it('lets an artist’s stage give way under whatever comes after it', () => {
		for (const to of ['artists', 'library', 'albums', 'album'] as const) {
			expect(choreographyFor('artist', to).held.artist?.to, to).toEqual({ opacity: 0 });
		}
	});

	it('re-presses an album in place: nothing travels, and the new stage comes up over the old', () => {
		const { travel, stages, held, arriving } = choreographyFor('album', 'album');
		expect(travel?.leads).toEqual([]);
		expect(stages.album?.from.opacity).toBe(0);
		expect(held.album?.to.opacity).toBe(0);
		expect(arriving.words?.from.y).toBeGreaterThan(0);
	});

	it('opens every record through its own door, the same from every room', () => {
		for (const from of ['home', 'library', 'albums', 'artist', 'playlist', 'search'] as const) {
			expect(choreographyFor(from, 'album').travel?.leads, from).toEqual([
				'cover:to',
				'title:to',
				'artist:to'
			]);
		}
		for (const from of ['artists', 'library', 'albums', 'search'] as const) {
			expect(choreographyFor(from, 'artist').name, from).toBe('name-door');
		}
		expect(choreographyFor('playlists', 'playlist').travel?.leads).toEqual([
			'mosaic:to',
			'listname:to'
		]);
	});

	it('reshapes the island exactly when Now playing is involved, and it was there to reshape', () => {
		for (const [from, to] of journeyPairs) {
			expect(choreographyFor(from, to).island !== null, `${from} → ${to}`).toBe(
				from !== 'login' && (from === 'now-playing' || to === 'now-playing')
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

	it('blends the pairs no milestone has taken', () => {
		for (const page of ['health', 'you', 'server'] as const) {
			expect(choreographyFor('album', page)).toBe(blend);
			expect(choreographyFor(page, 'album')).toBe(blend);
		}
		expect(choreographyFor('album', 'search')).toBe(blend);
		expect(choreographyFor('artist', 'artist')).toBe(blend);
	});
});

describe('the moment', () => {
	it('opens from every page the island shows on, from the island’s cover', () => {
		for (const page of islandPages.filter((each) => each !== 'album')) {
			const { name, travel, leaving, stages } = choreographyFor(page, 'now-playing');
			expect(name, page).toBe('into-now-playing');
			expect(travel).toMatchObject({ subject: 'playing', sourceSlots: ['island'] });
			// The one move allowed to fall back: the poster's stage comes up over it.
			expect(leaving.to.scale).toBeLessThan(1);
			expect(stages.poster?.from.opacity).toBe(0);
		}
		// From its album, the hero leaves when it is on screen.
		expect(choreographyFor('album', 'now-playing').travel?.sourceSlots).toEqual(['hero', 'island']);
	});

	it('goes back into the hand from every room and record: the poster lands where the page holds it, else in the thumb', () => {
		for (const page of islandPages.filter((each) => each !== 'album')) {
			const { travel, island, headline, otherwise, held } = choreographyFor('now-playing', page);
			expect(travel?.subject, page).toBe('playing');
			expect(travel?.sourceSlots).toEqual(['poster']);
			// Its own hero, a card in view, else the island's thumb, which is always there.
			expect(travel?.targetSlots).toEqual(['hero', 'card', 'island']);
			// On Home, the Continue card wins: the way out never retraces the way in.
			expect(travel?.landIn).toBe('feature');
			expect(island?.from.opacity).toBe(0);
			expect(held.poster?.to).toEqual({ opacity: 0 });
			// Built in front around the landing, the headline once the poster title has cleared.
			expect(otherwise?.gather).toBeDefined();
			if (page !== 'artist') {
				expect(headline?.arriving.delay).toBeGreaterThan(otherwise?.delay ?? 0);
			}
		}
	});

	it('flies only Play and the bar back with the controls’ pill: the rest go with the island', () => {
		expect(choreographyFor('now-playing', 'home').travel?.controls).toEqual([
			'control:pause',
			'progress'
		]);
		expect(choreographyFor('now-playing', 'album').travel?.controls).toEqual([
			'control:pause',
			'progress'
		]);
		// Into the moment every control rises with the glass, as it always has.
		expect(choreographyFor('home', 'now-playing').travel?.controls).toHaveLength(4);
	});

	it('is never the way in played backward', () => {
		for (const page of islandPages) {
			const into = choreographyFor(page, 'now-playing');
			const out = choreographyFor('now-playing', page);
			expect(out.name, page).not.toBe(into.name);
			expect(out.motion.token).toBe('--morph');
			expect(into.motion.token).toBe('--morph-slow');
			expect(out.leaving.to).toEqual({ opacity: 0 });
		}
	});

	it('lets the artist’s name take the stage, a beat after the poster title has gone', () => {
		const { travel, headline } = choreographyFor('now-playing', 'artist');
		expect(travel?.leads).toEqual(['name:to', 'face:to']);
		expect(travel?.waits?.name).toBe(40);
		expect(headline).toBeUndefined();
	});

	it('keeps the backdrop going to the artist: it is never held, and the cover’s light gives way', () => {
		const { held, stages } = choreographyFor('now-playing', 'artist');
		expect(held.artist).toBeUndefined();
		expect(held.poster?.to).toEqual({ opacity: 0 });
		// Only a stage that wasn't on screen comes up: the engine leaves one on both pages be.
		expect(stages.artist?.from).toEqual({ opacity: 0 });
		expect(choreographyFor('now-playing', 'home').held.artist?.to).toEqual({ opacity: 0 });
	});

	it('opens the door on the moment: no island to rise from, the controls close the frame', () => {
		const { name, travel, islandForms, arrivingByKey, arriving } = choreographyFor(
			'login',
			'now-playing'
		);
		expect(name).toBe('login-to-now-playing');
		expect(travel?.leads).toEqual([]);
		expect(islandForms?.from.y).toBeGreaterThan(0);
		expect(arrivingByKey?.['poster']?.from.y).toBe(48);
		// The title waits until the cover is solid: never seen through it.
		expect(arriving['line-1']?.delay).toBeGreaterThan(arrivingByKey?.['poster']?.delay ?? 0);
		expect(arriving['line-1']?.delay).toBe(360);
		expect(arriving['line-2']?.delay).toBe(450);
	});

	it('settles about 737 ms out, 1039 in, and 1259 through the door', () => {
		const lengths = {
			'--morph-fast': 280,
			'--morph': 420,
			'--morph-slow': 600,
			'--morph-drop': 180
		};
		const settles = (part: { delay: number; motion: Parameters<typeof resolveMotion>[0] }) =>
			part.delay + settleTime(resolveMotion(part.motion, (token) => lengths[token]));
		const out = choreographyFor('now-playing', 'home');
		expect(settles(out.headline?.arriving ?? out.leaving)).toBeGreaterThan(680);
		expect(settles(out.headline?.arriving ?? out.leaving)).toBeLessThan(760);
		const into = choreographyFor('home', 'now-playing').arriving['line-2'];
		expect(into === undefined ? 0 : settles(into)).toBeCloseTo(1039, -1);
		const door = choreographyFor('login', 'now-playing').arriving['line-2'];
		expect(door === undefined ? 0 : settles(door)).toBeCloseTo(1259, -1);
	});

	it('goes to the door with the Play button becoming the fox', () => {
		const { name, travel } = choreographyFor('now-playing', 'login');
		expect(name).toBe('door-closes-on-the-moment');
		expect(travel).toMatchObject({ subject: 'fox', standIn: 'control:pause', sourceSlots: [] });
	});
});

describe('the rooms, from the nav', () => {
	const roomPairs = [...rooms].flatMap((from) =>
		[...rooms].filter((to) => to !== from).map((to) => [from, to] as const)
	);
	const { headline, leaving, otherwise, arriving } = unfold;

	it('are the nav’s eight and the Server’s four, every pair of them a crossing that unfolds', () => {
		expect(rooms.size).toBe(12);
		expect(roomPairs).toHaveLength(132);
		for (const [from, to] of roomPairs) {
			const journey = choreographyFor(from, to);
			expect(journey, `${from} → ${to}`).toBe(unfold);
			expect(journey.grain).toBe('crossing');
		}
		for (const room of rooms) {
			expect(choreographyFor(room, room), `${room} → ${room}`).toBe(blend);
		}
	});

	it('lets the room left give way where it stands, quickly, and its headline first', () => {
		expect(leaving.to).toEqual({ opacity: 0 });
		expect(leaving.motion.token).toBe('--morph-fast');
		expect(headline?.leaving.to).toEqual({ opacity: 0 });
		expect(headline?.leaving.motion.pace ?? 1).toBeLessThan(leaving.motion.pace ?? 1);
	});

	it('bears its headline at the press at once, and builds the rest around it a beat later', () => {
		expect(headline?.arriving.born).toBe(true);
		expect(headline?.arriving.delay).toBe(0);
		expect(headline?.arriving.motion.token).toBe('--morph');
		expect(otherwise?.delay).toBe(40);
		expect(otherwise?.from.scale).toBeCloseTo(0.88);
		expect(otherwise?.gather).toBeDefined();
		// Every entrance at once: amplitude varies across the room, the start never does.
		expect(otherwise?.ripple).toBeUndefined();
		expect(arriving).toEqual({});
	});

	it('has no pose with a horizontal term, nor any travel of the page', () => {
		for (const part of [leaving, otherwise, headline?.arriving, headline?.leaving]) {
			expect(part?.from.y ?? 0).toBe(0);
			expect(part?.to.y ?? 0).toBe(0);
			expect(part?.from.rotate ?? 0).toBe(0);
		}
	});

	it('carries the playing album’s cover, and the field into Search, from what both rooms show', () => {
		expect(unfold.travel?.subject).toBe('rooms');
		expect(unfold.travel?.sourceSlots).toContain('nav');
		expect(unfold.travel?.targetSlots).not.toContain('nav');
		expect(unfold.bloom).toBeNull();
		expect(unfold.island).toBeNull();
	});

	it('settles about 607 ms after the press', () => {
		const settles =
			(otherwise?.delay ?? 0) +
			settleTime(resolveMotion(otherwise?.motion ?? leaving.motion, () => 420));
		expect(settles).toBeGreaterThan(560);
		expect(settles).toBeLessThan(660);
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

	it('closes from anywhere: the fox leaves the nav for the door, the frame dissolves where it stands', () => {
		for (const from of [
			...away,
			...rooms,
			'artist',
			'playlist',
			'duplicate',
			'matching'
		] as const) {
			const door = choreographyFor(from, 'login');
			expect(door.name, `${from} → login`).toMatch(/^door-closes/u);
			expect(door.travel?.subject).toBe('fox');
			expect(door.travel?.targetSlots).toEqual(['door']);
			// The frame on --morph, what the page says on --morph-fast, nothing travelling away.
			expect(door.frame?.to).toEqual({ opacity: 0 });
			expect(door.frame?.motion.token).toBe('--morph');
			expect(door.leaving.to).toEqual({ opacity: 0 });
			expect(door.leaving.motion.token).toBe('--morph-fast');
			// The door gathers in front around the fox, a beat after it leaves.
			expect(door.otherwise?.delay).toBe(90);
			expect(door.otherwise?.from.scale).toBeCloseTo(0.94);
			expect(door.otherwise?.gather).toBeDefined();
		}
		expect(choreographyFor('home', 'login').travel?.sourceSlots).toEqual(['nav']);
	});

	it('is not Login → Home played backward: nothing falls back, and the fox is the only path shared', () => {
		const out = choreographyFor('you', 'login');
		const into = choreographyFor('login', 'home');
		expect(out.leaving.to.scale).toBeUndefined();
		expect(into.leaving.to.scale).toBeLessThan(1);
		expect(out.islandForms).toBeUndefined();
	});

	it('opens again onto every page `next` names, by Login → Home’s parts', () => {
		for (const to of [
			'server',
			'trash',
			'organize',
			'duplicates',
			'duplicate',
			'matching',
			'you'
		] as const) {
			const through = choreographyFor('login', to);
			expect(through.name, `login → ${to}`).toBe('login-to-page');
			expect(through.travel).toEqual(choreographyFor('login', 'home').travel);
			expect(through.islandForms).toEqual(choreographyFor('login', 'home').islandForms);
			expect(through.motion.token).toBe('--morph-slow');
			expect(through.otherwise?.delay).toBe(160);
		}
		expect(choreographyFor('login', 'login')).toBe(blend);
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

describe('the Server pages and tools', () => {
	const tools = ['server', 'trash', 'organize', 'duplicates'] as const;
	const records = ['duplicate', 'matching'] as const;

	it('are rooms: each unfolds from the press into every room, and every room into each', () => {
		for (const tool of tools) {
			expect(rooms.has(tool)).toBe(true);
			for (const room of rooms) {
				if (room !== tool) {
					expect(choreographyFor(tool, room), `${tool} → ${room}`).toBe(unfold);
					expect(choreographyFor(room, tool), `${room} → ${tool}`).toBe(unfold);
				}
			}
		}
	});

	it('opens and leaves their records by the same unfold, where a lead may travel', () => {
		for (const record of records) {
			expect(rooms.has(record)).toBe(false);
			for (const room of rooms) {
				expect(choreographyFor(room, record), `${room} → ${record}`).toBe(unfold);
				expect(choreographyFor(record, room), `${record} → ${room}`).toBe(unfold);
			}
		}
		// The title leaves its row and the number its card for a headline, and come back to them.
		expect(unfold.travel?.sourceSlots).toEqual(expect.arrayContaining(['row', 'hero', 'card']));
		expect(unfold.travel?.targetSlots).toEqual(expect.arrayContaining(['row', 'hero', 'card']));
	});

	it('re-presses a group into its album pair in place, as Album → Album', () => {
		const pair = choreographyFor('duplicate', 'duplicate');
		expect(pair.name).toBe('duplicate-to-duplicate');
		expect(pair.grain).toBe('crossing');
		expect(pair.travel?.leads).toEqual([]);
		expect(pair.leaving.to).toEqual({ opacity: 0 });
		expect(pair.headline?.arriving.from.y).toBe(10);
		expect(pair.headline?.arriving.delay).toBe(90);
	});

	it('unfolds Organize from whatever was pressed for it, anywhere but the moment and the door', () => {
		for (const from of ['album', 'artist', 'playlist', 'home'] as const) {
			expect(choreographyFor(from, 'organize'), `${from} → organize`).toBe(unfold);
		}
		expect(choreographyFor('now-playing', 'organize').name).toBe('back-into-the-hand');
		expect(choreographyFor('login', 'organize').name).toBe('login-to-page');
	});

	it('opens an album from Matching by its door, the hero born at the link pressed', () => {
		const door = choreographyFor('matching', 'album');
		expect(door.name).toBe('album-door-from-press');
		expect(door.fromPress).toBe(true);
		expect(door.arriving.art?.born).toBe(true);
		expect(door.stages).toEqual(choreographyFor('albums', 'album').stages);
	});

	it('goes into the moment and back into the hand as every other page does', () => {
		for (const page of [...tools, ...records]) {
			expect(choreographyFor(page, 'now-playing').name).toBe('into-now-playing');
			expect(choreographyFor('now-playing', page).name).toBe('back-into-the-hand');
		}
	});
});
