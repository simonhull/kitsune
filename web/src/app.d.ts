// See https://svelte.dev/docs/kit/types#app.d.ts
// for information about these interfaces
declare global {
	namespace App {
		// interface Error {}
		// interface Locals {}
		// interface PageData {}
		interface PageState {
			/** The editor drawn over the page, whose entry Back takes off (`editorHistory`). */
			editor?: string;
		}
		// interface Platform {}
	}
}

export {};
