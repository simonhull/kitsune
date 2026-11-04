import type { OnNavigate } from '$app/navigation';
import { createVtEngine } from './engines/vt.ts';

export type EngineName = 'vt';

/** One way of running a choreography. Each engine owns its own after-step. */
export interface Engine {
	name: EngineName;
	/** Called from the one `onNavigate`. A returned promise holds the DOM update back. */
	navigate(navigation: OnNavigate): void | Promise<void>;
}

const vt = createVtEngine();

/** Returns the engine for this session. */
export function currentEngine(): Engine {
	return vt;
}
