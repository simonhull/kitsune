/** The query in a search page's URL. */
export function queryIn({ searchParams }: { searchParams: Pick<URLSearchParams, 'get'> }): string {
	return (searchParams.get('q') ?? '').trim();
}

/** The search part of a search page's URL for this text. Whatever else it says, it keeps. */
export function searchFor(text: string, current: string): string {
	const params = new URLSearchParams(current);
	const query = text.trim();
	if (query === '') {
		params.delete('q');
	} else {
		params.set('q', query);
	}
	const search = params.toString();
	return search === '' ? '' : `?${search}`;
}
