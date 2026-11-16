/**
 * The Organize page's state, kept here so a run outlives the page: the templates being typed, the
 * example songs, a preview and its pages, and the run in hand, followed by its `organize` events.
 * Nothing optimistic: moves are on disk and slow, so the page shows the run's progress as told.
 */

import { SvelteSet } from 'svelte/reactivity';
import { library } from '#lib/library/library.svelte.ts';
import type { FolderTemplates, ServerSettings, Setting } from '#lib/library/source.ts';
import { defaultCompilations, defaultTemplate } from './template.ts';
import {
	OrganizeError,
	type OrganizeAlbum,
	type OrganizeEvent,
	type OrganizeExample,
	type OrganizeFilter,
	type OrganizeLeft,
	type OrganizePreview,
	type OrganizeRequest,
	type OrganizeRun
} from './types.ts';

/** How long the search waits after the last key before asking. */
export const searchWaitMs = 200;
/** How often a page with no events asks how a run is going. */
const pollMs = 2000;

/** The three switches under the example, each a server setting. */
export type OrganizeOption =
	'organize.companions' | 'organize.on_edit' | 'organize.rewrite_playlists';

/** Where the right of the page is: the example, a preview, a run, or what a run did. */
export type OrganizeView = 'template' | 'preview' | 'run' | 'done';

const value = <T>(setting: Setting<T> | undefined, fallback: T): T => setting?.value ?? fallback;

class Organizer {
	template = $state(defaultTemplate);
	compilations = $state(defaultCompilations);
	/** What the server keeps: what Save template last saved. */
	saved = $state({ template: defaultTemplate, compilations: defaultCompilations });
	roots = $state.raw<string[]>([]);
	rootTemplates = $state.raw<Record<string, FolderTemplates>>({});
	options = $state<Record<OrganizeOption, boolean>>({
		'organize.companions': true,
		'organize.on_edit': false,
		'organize.rewrite_playlists': false
	});

	examples = $state.raw<OrganizeExample[]>([]);
	#seed = 0;

	view = $state<OrganizeView>('template');
	preview = $state.raw<OrganizePreview | null>(null);
	/** Every album the pages read so far, in order. */
	albums = $state.raw<OrganizeAlbum[]>([]);
	filter = $state<OrganizeFilter>('all');
	query = $state('');
	nextCursor = $state<string | undefined>(undefined);
	loading = $state(false);
	/** Albums picked to move on their own. Empty moves them all. */
	readonly picked = new SvelteSet<string>();
	problem = $state('');
	/** Where a template went wrong, by the server, when it refused one. */
	refused = $state<{ field: 'template' | 'compilations'; at: number; detail: string } | null>(null);

	run = $state.raw<OrganizeRun | null>(null);
	runs = $state.raw<OrganizeRun[]>([]);
	/** Albums finished, in the run's order, and the one in hand. */
	albumsDone = $state(0);
	runOrder = $state.raw<string[]>([]);
	left = $state.raw<OrganizeLeft[]>([]);
	/** When the run head first heard of this run, by this browser's clock, for its pace. */
	heardSince = $state(0);

	#asking = 0;
	#search: ReturnType<typeof setTimeout> | undefined;
	#poll: ReturnType<typeof setInterval> | undefined;
	#stopHearing: (() => void) | undefined;

	/** Starts hearing runs' progress, for the page and the nav. Idempotent. */
	hear(): void {
		this.#stopHearing ??= library.hearOrganize((event) => {
			this.heard(event);
		});
	}

	/** Stops hearing, at sign-out. */
	deafen(): void {
		this.#stopHearing?.();
		this.#stopHearing = undefined;
		clearInterval(this.#poll);
		this.#poll = undefined;
	}

	/** Takes in the settings the page's fields and switches show. */
	settled(settings: ServerSettings): void {
		const s = settings.settings;
		this.saved = {
			template: value(s['organize.template'], defaultTemplate),
			compilations: value(s['organize.compilations'], defaultCompilations)
		};
		this.template = this.saved.template;
		this.compilations = this.saved.compilations;
		this.roots = s['library.roots'].value;
		this.rootTemplates = value(s['organize.roots'], {});
		this.options = {
			'organize.companions': value(s['organize.companions'], true),
			'organize.on_edit': value(s['organize.on_edit'], false),
			'organize.rewrite_playlists': value(s['organize.rewrite_playlists'], false)
		};
	}

	/** Reads the settings, the runs and the examples: what the page opens on. */
	async open(): Promise<void> {
		this.hear();
		const [settings] = await Promise.allSettled([
			library.serverSettings(),
			this.readRuns(),
			this.another()
		]);
		if (settings.status === 'fulfilled') {
			this.settled(settings.value);
		}
		const going = this.runs.find((run) => run.state === 'running');
		if (going !== undefined) {
			// A reload finds the run it was following, and follows it again.
			this.follow(going);
		}
	}

	/** Other songs to show the template on. */
	async another(): Promise<void> {
		const seed = this.#seed;
		this.#seed += 3;
		try {
			this.examples = await library.organize.examples(3, seed);
		} catch {
			// The example is a help, never a gate: the page works without it.
		}
	}

	async readRuns(): Promise<void> {
		try {
			this.runs = await library.organize.runs();
		} catch {
			this.runs = [];
		}
	}

	/** Previews the templates being typed, over `scope` or the whole library. */
	async startPreview(scope: Pick<OrganizeRequest, 'tracks' | 'albums'> = {}): Promise<boolean> {
		const ask = ++this.#asking;
		this.loading = true;
		this.problem = '';
		this.refused = null;
		try {
			const preview = await library.organize.preview({
				template: this.template,
				compilations: this.compilations,
				...scope
			});
			if (ask === this.#asking) {
				this.showPreview(preview);
			}
			return true;
		} catch (error) {
			if (ask === this.#asking) {
				this.failed(error);
			}
			return false;
		} finally {
			if (ask === this.#asking) {
				this.loading = false;
			}
		}
	}

	/** Opens a preview made elsewhere: an album's or a selection's Organize. */
	async openPreview(id: string): Promise<void> {
		const ask = ++this.#asking;
		this.loading = true;
		try {
			const preview = await library.organize.page(id, { filter: 'all' });
			if (ask === this.#asking) {
				this.template = preview.template;
				this.compilations = preview.compilations;
				this.showPreview(preview);
			}
		} catch (error) {
			if (ask === this.#asking) {
				this.failed(error);
			}
		} finally {
			if (ask === this.#asking) {
				this.loading = false;
			}
		}
	}

	showPreview(preview: OrganizePreview): void {
		this.preview = preview;
		this.albums = preview.albums;
		this.nextCursor = preview.nextCursor;
		this.filter = preview.filter;
		this.query = '';
		this.picked.clear();
		if (this.view !== 'run') {
			this.view = 'preview';
		}
	}

	failed(error: unknown): void {
		if (error instanceof OrganizeError) {
			if (error.field !== undefined && error.at !== undefined) {
				this.refused = { field: error.field, at: error.at, detail: error.detail };
				return;
			}
			if (error.status === 404) {
				this.problem = 'That preview has gone: preview again.';
				this.leave();
				return;
			}
			this.problem =
				error.detail === '' ? 'The server didn’t answer. Try again.' : sentence(error.detail);
			return;
		}
		this.problem = 'The server didn’t answer. Try again.';
	}

	/** Back to the template and the example. */
	leave(): void {
		this.#asking += 1;
		this.loading = false;
		this.view = 'template';
		this.preview = null;
		this.albums = [];
		this.picked.clear();
	}

	/** Reads the first page of another filter or search. */
	async refilter(filter: OrganizeFilter, query = this.query): Promise<void> {
		const preview = this.preview;
		if (preview === null) {
			return;
		}
		this.filter = filter;
		this.query = query;
		const ask = ++this.#asking;
		this.loading = true;
		try {
			const page = await library.organize.page(preview.id, { filter, q: query });
			if (ask === this.#asking) {
				this.preview = page;
				this.albums = page.albums;
				this.nextCursor = page.nextCursor;
			}
		} catch (error) {
			if (ask === this.#asking) {
				this.failed(error);
			}
		} finally {
			if (ask === this.#asking) {
				this.loading = false;
			}
		}
	}

	/** The search, a moment after the last key. */
	search(query: string): void {
		this.query = query;
		clearTimeout(this.#search);
		this.#search = setTimeout(() => {
			void this.refilter(this.filter, query);
		}, searchWaitMs);
	}

	/** The next 200 albums, when the list nears its end. */
	async more(): Promise<void> {
		const preview = this.preview;
		const cursor = this.nextCursor;
		if (preview === null || cursor === undefined || this.loading) {
			return;
		}
		const ask = this.#asking;
		this.loading = true;
		try {
			const page = await library.organize.page(preview.id, {
				filter: this.filter,
				q: this.query,
				cursor
			});
			if (ask === this.#asking) {
				this.albums = [...this.albums, ...page.albums];
				this.nextCursor = page.nextCursor;
			}
		} catch (error) {
			if (ask === this.#asking) {
				this.failed(error);
			}
		} finally {
			if (ask === this.#asking) {
				this.loading = false;
			}
		}
	}

	/** How many songs Run it moves: the picked albums' moves, or every move. */
	get moving(): number {
		if (this.picked.size === 0) {
			return this.preview?.counts.moves ?? 0;
		}
		return this.albums
			.filter((album) => this.picked.has(album.id))
			.reduce(
				(sum, album) =>
					sum + album.files.filter((f) => f.kind === 'rename' || f.kind === 'move').length,
				0
			);
	}

	togglePick(id: string): void {
		if (this.picked.has(id)) {
			this.picked.delete(id);
		} else {
			this.picked.add(id);
		}
	}

	/** Runs the preview's moves, or the picked albums'. */
	async runIt(): Promise<void> {
		const preview = this.preview;
		if (preview === null) {
			return;
		}
		this.problem = '';
		const albums = this.picked.size === 0 ? undefined : [...this.picked];
		try {
			const run = await library.organize.start({
				previewId: preview.id,
				...(albums === undefined ? {} : { albums })
			});
			this.runOrder = this.albums
				.filter((album) => albums === undefined || albums.includes(album.id))
				.filter((album) => album.files.some((f) => f.kind === 'rename' || f.kind === 'move'))
				.map((album) => album.id);
			if (this.filter !== 'all') {
				void this.refilter('all', '');
			}
			this.follow(run);
		} catch (error) {
			this.failed(error);
		}
	}

	/** Follows a run: its head, its events, and a poll where no events come. */
	follow(run: OrganizeRun): void {
		this.run = run;
		this.albumsDone = 0;
		this.heardSince = Date.now();
		this.left = [];
		this.view = run.state === 'running' ? 'run' : 'done';
		this.#watch();
	}

	/** An `organize` event: the run in hand moves on, or ends. */
	heard(event: OrganizeEvent): void {
		const run = this.run;
		if (run === null || run.id !== event.run) {
			if (event.state === 'running') {
				// A run started elsewhere: the nav notes it, and the page follows it.
				this.run = {
					...blankRun(event.run),
					state: 'running',
					done: event.done,
					total: event.total
				};
				this.heardSince = Date.now();
				this.view = 'run';
				this.#watch();
			}
			return;
		}
		this.run = {
			...run,
			state: event.state,
			done: event.done,
			total: event.total,
			...(event.album === undefined ? {} : { album: event.album })
		};
		this.albumsDone = event.albumsDone;
		if (event.state !== 'running') {
			void this.ended(event.run);
		}
	}

	/** Reads what a run did once it ended, and the runs list. */
	async ended(id: string): Promise<void> {
		clearInterval(this.#poll);
		this.#poll = undefined;
		try {
			const detail = await library.organize.run(id);
			if (this.run?.id === id) {
				this.run = detail.run;
				this.left = detail.left;
			}
		} catch {
			// The head keeps what the events said.
		}
		this.view = 'done';
		this.preview = null;
		this.albums = [];
		await this.readRuns();
	}

	async stop(): Promise<void> {
		const run = this.run;
		if (run === null) {
			return;
		}
		try {
			this.run = await library.organize.stop(run.id);
		} catch (error) {
			this.failed(error);
		}
	}

	/** Puts back what a run moved: a run of its own, with the same head. */
	async undo(id: string): Promise<void> {
		this.problem = '';
		try {
			const undo = await library.organize.undo(id);
			this.runOrder = [];
			this.follow(undo);
		} catch (error) {
			this.failed(error);
		}
	}

	/** Saves the templates typed, and moves nothing. */
	async saveTemplates(): Promise<boolean> {
		this.problem = '';
		try {
			await library.changeSettings({
				'organize.template': this.template,
				'organize.compilations': this.compilations
			});
			this.saved = { template: this.template, compilations: this.compilations };
			return true;
		} catch (error) {
			this.problem =
				error instanceof Error && 'detail' in error && typeof error.detail === 'string'
					? sentence(error.detail)
					: 'The templates weren’t saved. Try again.';
			return false;
		}
	}

	/** Flips one of the three switches, saved at once. */
	async setOption(key: OrganizeOption, on: boolean): Promise<void> {
		const before = this.options[key];
		this.options[key] = on;
		try {
			await library.changeSettings({ [key]: on });
		} catch {
			this.options[key] = before;
			this.problem = 'That switch didn’t save. Try again.';
		}
	}

	/** A library folder's own templates, or none to use the main pair there. */
	async setRootTemplates(root: string, templates: FolderTemplates | null): Promise<void> {
		const others = Object.entries(this.rootTemplates).filter(([each]) => each !== root);
		const next = Object.fromEntries(
			templates === null ? others : [...others, [root, templates] as const]
		);
		try {
			await library.changeSettings({ 'organize.roots': next });
			this.rootTemplates = next;
		} catch {
			this.problem = 'That folder’s template didn’t save. Try again.';
		}
	}

	/** While a run goes and no events arrive, asks how it's going every couple of seconds. */
	#watch(): void {
		clearInterval(this.#poll);
		this.#poll = undefined;
		if (library.organize.listen !== undefined) {
			return; // The fixtures tell as they go.
		}
		this.#poll = setInterval(() => {
			if (library.live || this.run?.state !== 'running') {
				return;
			}
			void library.organize
				.runs()
				.then((runs) => {
					const now = runs.find((each) => each.id === this.run?.id);
					if (now !== undefined) {
						this.heard({
							run: now.id,
							state: now.state,
							done: now.done,
							total: now.total,
							albumsDone: this.albumsDone,
							...(now.album === undefined ? {} : { album: now.album })
						});
					}
				})
				.catch(() => undefined);
		}, pollMs);
	}
}

/** A run known only by an event, until its detail is read. */
function blankRun(id: string): OrganizeRun {
	return {
		id,
		kind: 'run',
		state: 'running',
		startedAt: new Date().toISOString(),
		template: '',
		total: 0,
		done: 0,
		moved: 0,
		folders: 0,
		conflicts: 0,
		left: 0,
		failed: 0,
		companions: 0,
		undoable: false
	};
}

/** The server's detail as a sentence: a capital, and a full stop. */
function sentence(detail: string): string {
	const text = detail.charAt(0).toUpperCase() + detail.slice(1);
	return /[.!?]$/.test(text) ? text : `${text}.`;
}

export const organizer = new Organizer();
