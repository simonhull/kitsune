/**
 * A template as the field draws it: fields as chips, folder breaks as dividers, brackets in the fox,
 * the rest as typed. It reads any text, a broken template too, so the field can draw what is being
 * typed and mark where it goes wrong; the parser is what says whether it's a template.
 */

/** One drawn piece of a template, from `start` to `end` in characters. */
export interface Part {
	kind: 'token' | 'slash' | 'open' | 'close' | 'literal';
	/** A token's name, as typed between its brackets; anything else's text. */
	text: string;
	start: number;
	end: number;
}

/** Splits a template into the pieces the field draws. */
export function partsOf(template: string): Part[] {
	const chars = Array.from(template);
	const out: Part[] = [];
	let at = 0;
	const literal = (c: string, start: number): void => {
		const last = out.at(-1);
		if (last?.kind === 'literal' && last.end === start) {
			last.text += c;
			last.end = start + 1;
		} else {
			out.push({ kind: 'literal', text: c, start, end: start + 1 });
		}
	};
	while (at < chars.length) {
		const c = chars[at] ?? '';
		if (c === '<') {
			const close = chars.indexOf('>', at);
			const body = close < 0 ? [] : chars.slice(at + 1, close);
			if (close >= 0 && !body.some((b) => '<[]/'.includes(b))) {
				out.push({ kind: 'token', text: body.join(''), start: at, end: close + 1 });
				at = close + 1;
				continue;
			}
		}
		if (c === '/' || c === '[' || c === ']') {
			out.push({
				kind: c === '/' ? 'slash' : c === '[' ? 'open' : 'close',
				text: c,
				start: at,
				end: at + 1
			});
		} else {
			literal(c, at);
		}
		at += 1;
	}
	return out;
}

/** The template without the part from `start` to `end`. */
export function without(template: string, part: Pick<Part, 'start' | 'end'>): string {
	const chars = Array.from(template);
	return [...chars.slice(0, part.start), ...chars.slice(part.end)].join('');
}

/** Whether a part holds the character at `at`, or, for a template too short, is the last. */
export function holds(part: Part, at: number, length: number): boolean {
	return at >= length ? part.end === length : at >= part.start && at < part.end;
}
