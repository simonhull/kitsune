import { describe, expect, it } from 'vitest';
import { cycled, repeatIn, repeatLabel } from './repeat.ts';

describe('repeat', () => {
	it('cycles off, all, this song, off', () => {
		expect(cycled('off', false)).toBe('all');
		expect(cycled('all', false)).toBe('one');
		expect(cycled('one', false)).toBe('off');
	});

	it('skips all in an endless shuffle, where all is off', () => {
		expect(cycled('off', true)).toBe('one');
		expect(cycled('one', true)).toBe('off');
		expect(repeatIn('all', true)).toBe('off');
		expect(cycled('all', true)).toBe('one');
		expect(repeatIn('one', true)).toBe('one');
	});

	it('says what it does in its name', () => {
		expect(repeatLabel('off', false)).toBe('Repeat: off');
		expect(repeatLabel('all', false)).toBe('Repeat: all');
		expect(repeatLabel('one', false)).toBe('Repeat: this song');
		expect(repeatLabel('all', true)).toBe('Repeat: off (an endless shuffle repeats only a song)');
		expect(repeatLabel('one', true)).toBe('Repeat: this song');
	});
});
