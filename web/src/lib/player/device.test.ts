import { describe, expect, it } from 'vitest';
import { deviceForm, deviceId, deviceKey } from './device.ts';

/** A storage of its own, in memory. */
function memory(): Storage {
	const held = new Map<string, string>();
	return {
		get length() {
			return held.size;
		},
		clear: () => {
			held.clear();
		},
		getItem: (key) => held.get(key) ?? null,
		key: (index) => [...held.keys()].at(index) ?? null,
		removeItem: (key) => {
			held.delete(key);
		},
		setItem: (key, value) => {
			held.set(key, value);
		}
	};
}

describe('deviceId', () => {
	it('makes an ID once and keeps it', () => {
		const store = memory();
		const id = deviceId(() => store);
		expect(id).toMatch(/^[0-9a-f]{32}$/);
		expect(store.getItem(deviceKey)).toBe(id);
		expect(deviceId(() => store)).toBe(id);
	});

	it('gives a per-load ID when storage is blocked', () => {
		const blocked = (): Storage => {
			throw new DOMException('blocked', 'SecurityError');
		};
		const id = deviceId(blocked);
		expect(id).toMatch(/^[0-9a-f]{32}$/);
		expect(deviceId(blocked)).toBe(id);
	});

	it('replaces a kept value that is not an ID', () => {
		const store = memory();
		store.setItem(deviceKey, 'nope');
		expect(deviceId(() => store)).toMatch(/^[0-9a-f]{32}$/);
	});
});

describe('deviceForm', () => {
	it.each([
		[false, 390, 844, 'computer'],
		[false, 1440, 900, 'computer'],
		[true, 390, 844, 'phone'],
		[true, 844, 390, 'phone'],
		[true, 599, 900, 'phone'],
		[true, 600, 900, 'tablet'],
		[true, 1024, 1366, 'tablet']
	] as const)('coarse %s at %i × %i is a %s', (coarse, width, height, form) => {
		expect(deviceForm(() => ({ coarse, width, height }))).toBe(form);
	});

	it('is a computer when the screen cannot be read', () => {
		expect(
			deviceForm(() => {
				throw new Error('no window');
			})
		).toBe('computer');
	});
});
