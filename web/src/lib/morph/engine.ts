import type { OnNavigate } from '$app/navigation';
import { linearEasing } from './choreography.ts';
import { createSpringEngine } from './engines/spring.ts';
import { createVtEngine } from './engines/vt.ts';
import { motionOf } from './journey.ts';

export const engineNames = ['vt', 'spring'] as const;

export type EngineName = (typeof engineNames)[number];

/** One way of running a choreography. Each engine owns its own after-step. */
export interface Engine {
	name: EngineName;
	/** Called from the one `onNavigate`. A returned promise holds the DOM update back. */
	navigate(navigation: OnNavigate): void | Promise<void>;
}

function isEngineName(value: string | null): value is EngineName {
	return engineNames.some((name) => name === value);
}

let engines: Record<EngineName, Engine> | null = null;
let chosen: EngineName = 'vt';

function mark(name: EngineName): void {
	document.documentElement.dataset['morphEngine'] = name;
}

/** Sets up both engines and picks one from `?engine=`, which link clicks then keep. */
export function startEngines(): void {
	engines = { vt: createVtEngine(), spring: createSpringEngine() };
	const asked = new URLSearchParams(location.search).get('engine');
	chosen = isEngineName(asked) ? asked : chosen;
	mark(chosen);
	// The shell's own small changes ride the same curve as the morphs around them.
	document.documentElement.style.setProperty(
		'--curve-morph',
		linearEasing(motionOf({ token: '--morph', bounce: 0.04 }))
	);
}

/** Returns the engine in use. */
export function currentEngine(): Engine {
	if (engines === null) {
		throw new Error('startEngines has not run');
	}
	return engines[chosen];
}

/** Switches engines for the following navigations. */
export function setEngine(name: EngineName): void {
	chosen = name;
	mark(name);
}
