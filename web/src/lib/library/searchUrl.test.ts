import { describe, expect, it } from 'vitest';
import { queryIn, searchFor } from './searchUrl';

describe('the search URL', () => {
	const current = '?source=fixtures&q=old';

	it('reads the query, trimmed', () => {
		expect(queryIn(new URL('http://kitsune.test/search?q=%20fox%20'))).toBe('fox');
		expect(queryIn(new URL('http://kitsune.test/search'))).toBe('');
	});

	it('writes the query and keeps the rest', () => {
		expect(searchFor(' red fox ', current)).toBe('?source=fixtures&q=red+fox');
	});

	it('drops the query when there is none', () => {
		expect(searchFor('  ', current)).toBe('?source=fixtures');
		expect(searchFor('', '?q=old')).toBe('');
	});

	it('survives whatever is typed', () => {
		const written = new URL(`http://kitsune.test/search${searchFor('a&b=c?#"é', current)}`);
		expect(queryIn(written)).toBe('a&b=c?#"é');
	});
});
