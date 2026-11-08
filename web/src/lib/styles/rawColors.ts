/**
 * Finds colors written into a component's styles instead of read from a token. A theme is one set
 * of token values, so a raw color is one that no theme can change.
 *
 * It reads `<style>` blocks and `style` attributes, not scripts: a cover's colors come from data,
 * through `style:` bindings, and are never written out. A line that must name a color anyway says
 * why with `raw-color:` in a comment on it or on the line above.
 */

/** A raw color, by its 1-based line. */
export interface RawColor {
	line: number;
	text: string;
}

const color = /#[\da-f]{3,8}\b|\b(?:rgba?|hsla?)\(/gi;
const styleBlock = /<style[^>]*>([\s\S]*?)<\/style>/g;
const styleAttribute = /\sstyle(?::[\w-]+)?=(?:"[^"]*"|'[^']*')/g;
const allow = 'raw-color:';

function lineAt(source: string, index: number): number {
	let line = 1;
	for (let at = source.indexOf('\n'); at !== -1 && at < index; at = source.indexOf('\n', at + 1)) {
		line += 1;
	}
	return line;
}

/** The regions of a file that are style: all of a `.css` file, or a `.svelte` file's styles. */
function styleRegions(source: string, css: boolean): [number, number][] {
	if (css) {
		return [[0, source.length]];
	}
	return [...source.matchAll(styleBlock), ...source.matchAll(styleAttribute)].map((match) => [
		match.index,
		match.index + match[0].length
	]);
}

/** Every raw color in a stylesheet (`css`) or a component, in order. */
export function rawColors(source: string, css = false): RawColor[] {
	const lines = source.split('\n');
	const found: RawColor[] = [];
	for (const [start, end] of styleRegions(source, css)) {
		for (const match of source.slice(start, end).matchAll(color)) {
			const line = lineAt(source, start + match.index);
			const here = lines.at(line - 1) ?? '';
			const above = lines.at(line - 2) ?? '';
			if (!here.includes(allow) && !above.includes(allow)) {
				found.push({ line, text: here.trim() });
			}
		}
	}
	return found.sort((a, b) => a.line - b.line);
}
