import { describe, expect, it } from 'vitest';
import { viewTransitionName } from './vt';

describe('viewTransitionName', () => {
	it('sanitizes and prefixes, so a digit never leads', () => {
		expect(viewTransitionName('cover:01J8ZQ')).toBe('m-cover-01J8ZQ');
		expect(viewTransitionName('9 lives/é')).toBe('m-9-lives--');
	});
});
