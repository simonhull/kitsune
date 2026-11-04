/** Formats seconds as m:ss. */
export function clock(totalSeconds: number): string {
	const minutes = Math.floor(totalSeconds / 60);
	const seconds = Math.floor(totalSeconds % 60);
	return `${String(minutes)}:${String(seconds).padStart(2, '0')}`;
}

/** Formats ReplayGain the way the boards do, with a true minus sign. */
export function gain(decibels: number): string {
	const magnitude = Math.abs(decibels).toFixed(1);
	return decibels < 0 ? `−${magnitude}` : `+${magnitude}`;
}

/** Splits a title into two lines of similar length, at a word boundary. */
export function balancedLines(title: string): [string, string] {
	const words = title.split(' ');
	let best: [string, string] = [title, ''];
	let bestLongest = title.length;
	for (let cut = 1; cut < words.length; cut += 1) {
		const first = words.slice(0, cut).join(' ');
		const second = words.slice(cut).join(' ');
		const longest = Math.max(first.length, second.length);
		if (longest < bestLongest) {
			best = [first, second];
			bestLongest = longest;
		}
	}
	return best;
}
