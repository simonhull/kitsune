/** Used with no canvas to measure on, and a fair average for Geist at these weights. */
const averageEm = 0.58;
/** Room for the difference between a measurement and a line of real glyphs. */
const margin = 1.03;

const widths = new Map<string, number>();
let pen: CanvasRenderingContext2D | null | undefined;

function measureEm(text: string, weight: number, tracking: number): number {
	pen ??=
		typeof document === 'undefined' ? null : document.createElement('canvas').getContext('2d');
	if (pen === null || pen === undefined) {
		return text.length * averageEm;
	}
	pen.font = `${String(weight)} 100px "Geist Variable", system-ui, sans-serif`;
	return (pen.measureText(text).width / 100 + text.length * tracking) * margin;
}

/**
 * How wide a line of text is in ems, for headlines sized to fit. It reads the font's own advances
 * instead of the page, so the first frame is final: nothing lays out, and nothing jumps. The widest
 * words in real titles run far past any per-character average, and a fit that guesses cuts them off.
 */
export function emWidth(text: string, weight: number, tracking: number): number {
	const key = `${String(weight)}/${String(tracking)}/${text}`;
	const known = widths.get(key);
	if (known !== undefined) {
		return known;
	}
	const width = measureEm(text, weight, tracking);
	widths.set(key, width);
	return width;
}
