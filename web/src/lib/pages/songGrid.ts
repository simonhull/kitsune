/**
 * A song list's grid, as a style rule of its own: `[data-song-grid='3'] :is(rows, headers) {
 * grid-template-columns: … }`, in one constructed sheet, written through the CSSOM. A custom
 * property on the list did the same, but every element under the list inherits it, so a new order
 * restyled the whole Library (about 1,400 elements on a 1920 screen) where only its rows and headers
 * lay out anew. A rule restyles just the elements it matches, and the property isn't inherited, so
 * their cells are left alone (plan 052's Outcome, and its follow-up).
 */
import type { Action } from 'svelte/action';

let sheet: CSSStyleSheet | null = null;
let lists = 0;

function ownSheet(): CSSStyleSheet {
	if (sheet === null) {
		sheet = new CSSStyleSheet();
		document.adoptedStyleSheets = [...document.adoptedStyleSheets, sheet];
	}
	return sheet;
}

/** The rule for the list with `key`: its rows and its headers, however deep they sit. */
export function gridRule(key: string, template: string): string {
	return `[data-song-grid='${key}'] :is(.row[data-track], .header[data-song-header]) { grid-template-columns: ${template}; }`;
}

/** Gives the list's rows and headers `template`, and a new one whenever it changes. */
export const songGrid: Action<HTMLElement, string> = (list, template) => {
	const key = String((lists += 1));
	const own = ownSheet();
	own.insertRule(gridRule(key, template), own.cssRules.length);
	const rule = own.cssRules.item(own.cssRules.length - 1);
	list.dataset['songGrid'] = key;
	let current = template;
	return {
		update(next) {
			if (next !== current && rule instanceof CSSStyleRule) {
				current = next;
				rule.style.setProperty('grid-template-columns', next);
			}
		},
		destroy() {
			const at = [...own.cssRules].indexOf(rule as CSSRule);
			if (at >= 0) {
				own.deleteRule(at);
			}
			delete list.dataset['songGrid'];
		}
	};
};
