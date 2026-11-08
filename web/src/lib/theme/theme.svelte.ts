/** A set of values for the color tokens, as `tokens.css` names them under `data-theme`. */
export type Theme = 'dark' | 'light';

/** What a person picks: a theme, or whatever the system shows. */
export type ThemeChoice = 'system' | Theme;

/** The choices, in the order the You page shows them. */
export const themeChoices: readonly ThemeChoice[] = ['system', 'light', 'dark'];

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

function cache(choice: ThemeChoice | null): void {
	try {
		if (choice === null) {
			localStorage.removeItem(themeCacheKey);
		} else {
			localStorage.setItem(themeCacheKey, choice);
		}
	} catch {
		// Storage can be off: the next load follows the system until the server answers.
	}
}

/**
 * The theme on screen. It starts from what `app.html` read, so the first frame and this agree. A
 * change after that crossfades on the root, unless motion is reduced.
 */
class ThemeState {
	/** Null when nobody is signed in, or before the person's choice is known. */
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
			this.#paint(true);
		};
		media.addEventListener('change', changed);
		this.#paint(false);
		return () => {
			media.removeEventListener('change', changed);
		};
	}

	/** Takes a choice, the person's here or the one the server kept, and keeps it for the next load. */
	choose(choice: ThemeChoice): void {
		this.choice = choice;
		cache(choice);
		this.#paint(true);
	}

	/** Forgets the choice when the session ends: the door follows the system. */
	forget(): void {
		this.choice = null;
		cache(null);
		this.#paint(true);
	}

	#paint(fade: boolean): void {
		if (typeof document === 'undefined') {
			return;
		}
		const root = document.documentElement;
		const next = this.current;
		if (root.dataset.theme === next) {
			return;
		}
		const swap = (): void => {
			root.dataset.theme = next;
			// The browser's own chrome, on a phone, takes the ground too.
			const ground = getComputedStyle(root).getPropertyValue('--ground').trim();
			document.querySelector('meta[name="theme-color"]')?.setAttribute('content', ground);
		};
		const still = matchMedia('(prefers-reduced-motion: reduce)').matches;
		// Firefox before 144 has no view transitions: it changes at once.
		if (!fade || still || typeof document.startViewTransition !== 'function') {
			swap();
			return;
		}
		// A picture of the page crossfades into the next (base.css), so no element's own transitions
		// are touched.
		root.classList.add('theme-fading');
		void document
			.startViewTransition(swap)
			.finished.catch(() => undefined)
			.finally(() => {
				root.classList.remove('theme-fading');
			});
	}
}

export const theme = new ThemeState();
