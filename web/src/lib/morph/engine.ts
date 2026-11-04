import type { OnNavigate } from '$app/navigation';
import { linearEasing } from './choreography.ts';
import { createSpringEngine } from './engines/spring.ts';
import { motionOf } from './journey.ts';

/** Starts the morphs, and returns what the one `onNavigate` calls. */
export function startMorphs(): (navigation: OnNavigate) => void {
	// The shell's own small changes ride the same curve as the morphs around them.
	document.documentElement.style.setProperty(
		'--curve-morph',
		linearEasing(motionOf({ token: '--morph', bounce: 0.04 }))
	);
	return createSpringEngine();
}
