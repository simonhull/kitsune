import type { Page } from '@playwright/test';
import { expect, landed } from './fixture';

/**
 * What every journey's tests share: the slow-motion clock, what each navigation draws under the page
 * and in the air, the In-Front Rule as the shell is built, landing, and turning back mid-flight.
 */

/**
 * A slow-motion knob for the page's clock, installed before the page loads: `performance.now` and the frame timestamps run at
 * `1 / factor`, never backward, so a journey can be caught in mid-flight however loaded the machine
 * is. Nothing in the app changes; the springs simply see less time pass.
 */
export async function slowClock(page: Page): Promise<void> {
	await page.addInitScript(() => {
		const real = performance.now.bind(performance);
		let base = real();
		let since = base;
		let factor = 1;
		const now = () => base + (real() - since) / factor;
		performance.now = now;
		const frame = window.requestAnimationFrame.bind(window);
		window.requestAnimationFrame = (callback) => frame(() => callback(now()));
		Object.assign(window, {
			slowMotion(next: number) {
				base = now();
				since = real();
				factor = next;
			}
		});
	});
}

/** Runs the page's motion `factor` times slower, from now on. */
export async function slowMotion(page: Page, factor: number): Promise<void> {
	await page.evaluate((next) => {
		(window as unknown as { slowMotion: (factor: number) => void }).slowMotion(next);
	}, factor);
}

export const nav = (page: Page, name: string) =>
	page.locator('nav[aria-label="Main"]').getByRole('link', { name, exact: true });

/** Starts noting what each navigation puts under the page and in the air. */
export async function watch(page: Page): Promise<void> {
	await page.evaluate(() => {
		const seen = { ghostPages: 0, airPages: 0, pictures: 0, journeys: [] as string[] };
		Object.assign(window, { journeySeen: seen });
		const ghost = document.querySelector('.ghost-layer');
		const air = document.querySelector('.morph-layer');
		new MutationObserver((changes) => {
			for (const change of changes) {
				for (const node of change.addedNodes) {
					if (!(node instanceof HTMLElement)) continue;
					// A picture of the page left, or of everything for a blend.
					const picture = node.dataset['morphGhost'];
					if (change.target === ghost && picture === 'page') {
						seen.ghostPages += 1;
						seen.journeys.push(node.dataset['morphJourney'] ?? '');
					}
					if (change.target === ghost && picture === 'everything') seen.pictures += 1;
					if (change.target === air && picture !== undefined) seen.airPages += 1;
				}
			}
		}).observe(document.body, { childList: true, subtree: true });
	});
}

/** What the last navigations drew under the page and in the air, since `watch`. */
/** What a watched page saw: pictures under it and in the air, and the journey that left each. */
export interface Seen {
	ghostPages: number;
	airPages: number;
	pictures: number;
	journeys: string[];
}

export function seen(page: Page): Promise<Seen> {
	return page.evaluate(() => (window as unknown as { journeySeen: Seen }).journeySeen);
}

/**
 * The In-Front Rule, as the shell is built: the ghost layer is painted under the stages and the
 * page (the page you go to is always in front), and the air over all of them.
 */
export async function expectInFront(page: Page): Promise<void> {
	const order = await page.evaluate(() => {
		const shell = document.querySelector('.shell');
		const ghost = shell?.querySelector(':scope > .ghost-layer');
		const main = shell?.querySelector(':scope > main');
		const backdrop = shell?.querySelector(':scope > .backdrop');
		const air = shell?.querySelector(':scope > .morph-layer');
		const born = shell?.querySelector(':scope > .born-layer');
		const bar = shell?.querySelector(':scope > header.bar');
		const z = (element: Element | null | undefined) =>
			element === null || element === undefined ? null : Number(getComputedStyle(element).zIndex);
		const before = (a: Element | null | undefined, b: Element | null | undefined) =>
			a !== null &&
			a !== undefined &&
			b !== null &&
			b !== undefined &&
			(a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;
		return {
			ghost: z(ghost),
			main: z(main),
			air: z(air),
			ghostBeforeMain: before(ghost, main),
			ghostBeforeStage: backdrop === null || backdrop === undefined || before(ghost, backdrop),
			stage: z(backdrop),
			born: z(born),
			bar: z(bar)
		};
	});
	expect(order.ghost).toBe(0);
	expect(order.main).toBeGreaterThan(order.ghost ?? 0);
	expect(order.ghostBeforeMain).toBe(true);
	// The stage of the page arrived at is over the page left, which is under the stage it had.
	expect(order.ghostBeforeStage).toBe(true);
	expect(order.stage ?? 0).toBeGreaterThanOrEqual(order.ghost ?? 0);
	expect(order.air).toBeGreaterThan(order.main ?? 0);
	// What a room is born from in the nav comes out over the page and under the bar's glass.
	expect(order.born).toBeGreaterThan(order.main ?? 0);
	expect(order.bar).toBeGreaterThan(order.born ?? 0);
}

export const expectLanded = landed;

/**
 * Plays a journey and holds it to the rule: the page left is drawn under the page arriving, never
 * over it, and the journey lands.
 */
export async function journey(page: Page, go: () => Promise<void>, to: RegExp): Promise<void> {
	await watch(page);
	await go();
	await expect(page).toHaveURL(to);
	await expectInFront(page);
	await expectLanded(page);
	const { ghostPages, airPages } = await seen(page);
	expect(ghostPages).toBeGreaterThan(0);
	expect(airPages).toBe(0);
}

/**
 * Plays a journey and goes back once something is in the air: what flies must turn where it is,
 * the same element carrying on to where it came from, and the page it left must settle again.
 */
export async function turnsBack(
	page: Page,
	go: () => Promise<void>,
	back: () => Promise<unknown> = () => page.evaluate(() => history.back())
): Promise<void> {
	// From the page itself: Playwright hears of a history entry a moment after the page does.
	const before = await page.evaluate(() => location.pathname);
	// Slowed, so the turn always comes while something is still in the air.
	await slowMotion(page, 20);
	await go();
	// Under way: the page has changed, and what it shares is in the air.
	await page.waitForFunction(
		(from) =>
			location.pathname !== from &&
			(document.querySelector('.shell > .morph-layer')?.children.length ?? 0) > 0,
		before
	);
	// And moving: a flier can wait before it sets off, and one turned while still on its mark is
	// already where it would land, so it lands at once. Only one that has travelled can turn: moved
	// or grown, measured by what it draws, since a flier moves the pieces inside it.
	await page.evaluate(() => {
		const drawn = (flier: Element): number[] => {
			const boxes = [flier, ...flier.querySelectorAll('*')]
				.map((each) => each.getBoundingClientRect())
				.filter((box) => box.width > 0 || box.height > 0);
			return [
				Math.min(...boxes.map((box) => box.left)),
				Math.min(...boxes.map((box) => box.top)),
				Math.max(...boxes.map((box) => box.right)),
				Math.max(...boxes.map((box) => box.bottom))
			];
		};
		const fliers = () => [...(document.querySelector('.shell > .morph-layer')?.children ?? [])];
		Object.assign(window, {
			flightDrawn: drawn,
			flightFliers: fliers,
			flightStart: fliers().map(drawn)
		});
	});
	await page.waitForFunction(() => {
		const { flightDrawn, flightFliers, flightStart } = window as unknown as {
			flightDrawn: (flier: Element) => number[];
			flightFliers: () => Element[];
			flightStart: number[][];
		};
		const now = flightFliers();
		if (now.length < flightStart.length) {
			return true; // One has landed already: the check below says whether any is left.
		}
		return now.some((flier, at) => {
			const was = flightStart[at] ?? [];
			return flightDrawn(flier).some((edge, side) => Math.abs(edge - (was[side] ?? edge)) > 4);
		});
	});
	await page.evaluate(() => {
		for (const flier of document.querySelector('.shell > .morph-layer')?.children ?? []) {
			flier.setAttribute('data-turned', '');
		}
	});
	await back();
	await page.waitForFunction((path) => location.pathname === path, before);
	// Still the same fliers, now on their way back: a snap would have removed them.
	expect(await page.locator('.shell > .morph-layer > [data-turned]').count()).toBeGreaterThan(0);
	await slowMotion(page, 1);
	await expectLanded(page);
	expect(await page.evaluate(() => location.pathname)).toBe(before);
}

/** Goes somewhere the plain way, and waits for the journey there to land. */
export async function arrive(page: Page, go: () => Promise<void>, to: RegExp): Promise<void> {
	await go();
	await page.waitForFunction((pattern) => new RegExp(pattern).test(location.pathname), to.source);
	await expectLanded(page);
}
