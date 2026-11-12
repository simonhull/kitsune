import { newClientId } from '#lib/library/outbox.ts';
import type { DeviceForm } from '#lib/library/source.ts';

/** Where this browser keeps its own random ID: a convenience of the browser, not the record. */
export const deviceKey = 'kitsune.device';

/** A screen whose shorter side is under this, with a coarse pointer, is a phone's. */
const phoneSide = 600;

/** The ID used when storage is blocked: one per load. */
let perLoad: string | null = null;

/**
 * This browser's own ID, which a remembered record names to say whether it was saved here. It is
 * kept in storage; with storage blocked, it's one per load.
 */
export function deviceId(store: () => Storage = () => localStorage): string {
	try {
		const storage = store();
		const kept = storage.getItem(deviceKey);
		if (kept !== null && /^[0-9a-f]{16,64}$/.test(kept)) {
			return kept;
		}
		const made = newClientId();
		storage.setItem(deviceKey, made);
		return made;
	} catch {
		perLoad ??= newClientId();
		return perLoad;
	}
}

/** What a browser shows of its device: whether its pointer is coarse, and its screen's sides. */
export interface Screen {
	coarse: boolean;
	width: number;
	height: number;
}

/** The screen this page runs on. */
function thisScreen(): Screen {
	return {
		coarse: matchMedia('(pointer: coarse)').matches,
		width: screen.width,
		height: screen.height
	};
}

/**
 * The coarse form of this browser's device: a phone (a coarse pointer and a shorter side under
 * 600), a tablet (coarse, and larger), or a computer. No user agent is read: they're frozen and lie.
 */
export function deviceForm(on: () => Screen = thisScreen): DeviceForm {
	try {
		const { coarse, width, height } = on();
		if (!coarse) {
			return 'computer';
		}
		return Math.min(width, height) < phoneSide ? 'phone' : 'tablet';
	} catch {
		return 'computer';
	}
}
