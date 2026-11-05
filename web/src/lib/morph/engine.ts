import { linearEasing } from './choreography.ts';
import { createSpringEngine, type Morphs } from './engines/spring.ts';
import { inked, printTitle, wipePlates } from './ink.ts';
import { motionOf } from './journey.ts';

/** Where a page below the shell finds the engine, through Svelte's context. */
export const morphsKey = Symbol('morphs');

/** Starts the morphs. */
export function startMorphs(): Morphs {
	// The shell's own small changes ride the same curve as the morphs around them.
	document.documentElement.style.setProperty(
		'--curve-morph',
		linearEasing(motionOf({ token: '--morph', bounce: 0.04 }))
	);
	if (!inked) {
		return createSpringEngine();
	}
	document.documentElement.dataset['ink'] = '';
	const morphs = createSpringEngine(printTitle);
	return {
		...morphs,
		navigate(navigation) {
			wipePlates();
			morphs.navigate(navigation);
		}
	};
}
