/** A set of values for the color tokens, as `tokens.css` names them under `data-theme`. */
export type Theme = 'dark' | 'light';

/** What a person picks: a theme, or whatever the system shows. */
export type ThemeChoice = 'system' | Theme;

/**
 * Where a signed-in browser keeps the person's choice, so the page paints in it before any script
 * loads. `app.html` reads it too, with the same rule as {@link resolveTheme}: keep the two together.
 */
export const themeCacheKey = 'kitsune.theme';

const systemLight = '(prefers-color-scheme: light)';

export function isThemeChoice(value: unknown): value is ThemeChoice {
	return value === 'system' || value === 'light' || value === 'dark';
}

/**
 * The theme to paint. A choice of Light or Dark is kept; System, or nobody signed in, follows the
 * system, and a system with no preference gets Dark, Kitsune's first look.
 */
export function resolveTheme(choice: ThemeChoice | null, prefersLight: boolean): Theme {
	if (choice === 'light' || choice === 'dark') {
		return choice;
	}
	return prefersLight ? 'light' : 'dark';
}

function cached(): ThemeChoice | null {
	try {
		const value = localStorage.getItem(themeCacheKey);
		return isThemeChoice(value) ? value : null;
	} catch {
		return null; // Storage can be off; the system decides then.
	}
}

/** The theme on screen. It reads what `app.html` read, so the first frame and this agree. */
class ThemeState {
	choice = $state<ThemeChoice | null>(null);
	prefersLight = $state(false);
	readonly current: Theme = $derived(resolveTheme(this.choice, this.prefersLight));

	constructor() {
		if (typeof window === 'undefined') {
			return; // Tests and the build's route analysis: no page to paint.
		}
		this.choice = cached();
		this.prefersLight = matchMedia(systemLight).matches;
	}

	/** Follows the system while the choice leaves it to the system. Returns the way to stop. */
	follow(): () => void {
		const media = matchMedia(systemLight);
		const changed = (event: MediaQueryListEvent): void => {
			this.prefersLight = event.matches;
			this.#paint();
		};
		media.addEventListener('change', changed);
		this.#paint();
		return () => {
			media.removeEventListener('change', changed);
		};
	}

	#paint(): void {
		const root = document.documentElement;
		root.dataset.theme = this.current;
		// The browser's own chrome, on a phone, takes the ground too.
		const ground = getComputedStyle(root).getPropertyValue('--ground').trim();
		document.querySelector('meta[name="theme-color"]')?.setAttribute('content', ground);
	}
}

export const theme = new ThemeState();
