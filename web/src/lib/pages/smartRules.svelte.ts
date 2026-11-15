/**
 * The rules editor's draft: a smart playlist's rules as they're being changed, beside the rules as
 * saved, and what they choose, asked of the server as they change. The editor that holds a draft has
 * a history entry of its own (`editorHistory`): Back and Esc are Leave it, and the draft never
 * outlives the editor.
 */
import { freshRules, isGroup, newSmartName, ruleOf, sameRules } from '#lib/library/smartlist.ts';
import type {
	SmartChooser,
	SmartField,
	SmartLimit,
	SmartOrder,
	SmartPreview,
	SmartPreviewRequest,
	SmartRule,
	SmartRules
} from '#lib/library/source.ts';

/** How long a change waits for the next before the preview is asked. */
export const previewDelayMs = 120;

/** A rule as the editor holds it: with a key, so its row keeps its focus as others come and go. */
export interface DraftRule {
	key: number;
	rule: SmartRule;
}

/** A group as the editor holds it. */
export interface DraftGroup {
	key: number;
	match: 'all' | 'any';
	rules: DraftRule[];
}

export type DraftItem = DraftRule | DraftGroup;

export function isDraftGroup(item: DraftItem): item is DraftGroup {
	return 'rules' in item;
}

/** A limit as the editor holds it: its values stay while it's off, so turning it on brings them back. */
export interface DraftLimit extends SmartLimit {
	on: boolean;
}

const offLimit: DraftLimit = { on: false, count: 50, unit: 'songs', by: 'random' };

/** A new draft's seed: what its random choices follow, sent with its previews and its save. */
export function newSeed(): number {
	return Math.floor(Math.random() * Number.MAX_SAFE_INTEGER);
}

/** Where a rule is: the top level's `at`, or the `in`th rule of the group at `at`. */
export interface RulePlace {
	at: number;
	in?: number;
}

export class SmartDraft {
	name = $state('');
	match = $state<'all' | 'any'>('all');
	items = $state<DraftItem[]>([]);
	limit = $state<DraftLimit>({ ...offLimit });
	order = $state<SmartOrder>('library');
	/** The rules as saved, which the dock and the card compare with. */
	readonly saved: SmartRules;
	readonly savedName: string;
	/** A draft of a playlist not yet made: it's never "Nothing changed". */
	readonly unsaved: boolean;
	#keys = 0;

	constructor(rules: SmartRules, name: string, unsaved = false) {
		this.unsaved = unsaved;
		this.saved = rules;
		this.savedName = name;
		this.name = name;
		this.match = rules.match;
		this.items = rules.rules.map((c) =>
			isGroup(c)
				? { key: this.#key(), match: c.match, rules: c.rules.map((rule) => this.#keyed(rule)) }
				: this.#keyed(c)
		);
		this.limit = rules.limit === undefined ? { ...offLimit } : { on: true, ...rules.limit };
		this.order = rules.order ?? 'library';
	}

	/** A fresh draft: "New smart playlist", one rule, Rating is at least ★★★★. */
	static fresh(): SmartDraft {
		return new SmartDraft(freshRules(), newSmartName, true);
	}

	#key(): number {
		this.#keys += 1;
		return this.#keys;
	}

	#keyed(rule: SmartRule): DraftRule {
		return { key: this.#key(), rule };
	}

	/** The draft as rules, as the server takes them. */
	get rules(): SmartRules {
		const { on, ...limit } = $state.snapshot(this.limit);
		return $state.snapshot({
			match: this.match,
			rules: this.items.map((item) =>
				isDraftGroup(item)
					? { match: item.match, rules: item.rules.map(({ rule }) => rule) }
					: item.rule
			),
			...(on ? { limit } : {}),
			order: this.order
		});
	}

	/** Whether anything differs from what's saved, the name included. */
	get changed(): boolean {
		return (
			this.unsaved || !sameRules(this.rules, this.saved) || this.name.trim() !== this.savedName
		);
	}

	/** Whether a group may be added: there's one level of group, and one group. */
	get canGroup(): boolean {
		return !this.items.some(isDraftGroup);
	}

	ruleAt(place: RulePlace): SmartRule | undefined {
		const item = this.items[place.at];
		if (item === undefined) {
			return undefined;
		}
		if (place.in === undefined) {
			return isDraftGroup(item) ? undefined : item.rule;
		}
		return isDraftGroup(item) ? item.rules[place.in]?.rule : undefined;
	}

	/** Changes a rule in place. */
	patch(place: RulePlace, change: Partial<SmartRule>): void {
		this.items = this.items.map((item, at) => {
			if (at !== place.at) {
				return item;
			}
			if (place.in === undefined) {
				return isDraftGroup(item) ? item : { ...item, rule: { ...item.rule, ...change } };
			}
			return isDraftGroup(item)
				? {
						...item,
						rules: item.rules.map((each, i) =>
							i === place.in ? { ...each, rule: { ...each.rule, ...change } } : each
						)
					}
				: item;
		});
	}

	/** A rule takes a new field: its first operator, and a starting value. */
	setField(place: RulePlace, field: SmartField, picks: readonly string[] = []): void {
		const rule = ruleOf(field, picks);
		this.patch(place, rule);
	}

	addRule(picks: readonly string[] = []): void {
		this.items = [...this.items, this.#keyed(ruleOf('rating', picks))];
	}

	/** A group of any, with one rule to fill in. */
	addGroup(): void {
		if (!this.canGroup) {
			return;
		}
		this.items = [
			...this.items,
			{ key: this.#key(), match: 'any', rules: [this.#keyed(ruleOf('album'))] }
		];
	}

	addToGroup(at: number): void {
		this.items = this.items.map((item, i) =>
			i === at && isDraftGroup(item)
				? { ...item, rules: [...item.rules, this.#keyed(ruleOf('album'))] }
				: item
		);
	}

	setGroupMatch(at: number, match: 'all' | 'any'): void {
		this.items = this.items.map((item, i) =>
			i === at && isDraftGroup(item) ? { ...item, match } : item
		);
	}

	/** Takes out a rule, or a whole group; a group left with no rule goes too. */
	remove(place: RulePlace): void {
		if (place.in === undefined) {
			this.items = this.items.filter((_, i) => i !== place.at);
			return;
		}
		this.items = this.items
			.map((item, i) =>
				i === place.at && isDraftGroup(item)
					? { ...item, rules: item.rules.filter((_, j) => j !== place.in) }
					: item
			)
			.filter((item) => !isDraftGroup(item) || item.rules.length > 0);
	}

	/** Turns the limit on or off. On, the order becomes how they were chosen; off, that order goes. */
	toggleLimit(): void {
		const on = !this.limit.on;
		this.limit = { ...this.limit, on };
		if (on) {
			this.order = 'chosen';
		} else if (this.order === 'chosen') {
			this.order = 'library';
		}
	}

	setLimit(change: Partial<SmartLimit>): void {
		this.limit = { ...this.limit, ...change };
	}

	setChooser(by: SmartChooser): void {
		this.setLimit({ by });
	}
}

/** What asks the server what rules choose. */
export type AskPreview = (
	request: SmartPreviewRequest,
	signal: AbortSignal
) => Promise<SmartPreview>;

/**
 * What a draft's rules choose, asked as they change: each change waits {@link previewDelayMs} for
 * the next, and a question sent aborts the one before. The answer stays until the next arrives, so
 * the card never blanks.
 */
export class LivePreview {
	answer = $state.raw<SmartPreview | null>(null);
	/** The last question failed: the card keeps the answer before it. */
	failed = $state(false);
	readonly #ask: AskPreview;
	readonly #delay: number;
	#timer: ReturnType<typeof setTimeout> | undefined;
	#controller: AbortController | null = null;

	constructor(ask: AskPreview, delay = previewDelayMs) {
		this.#ask = ask;
		this.#delay = delay;
	}

	/** Asks what these rules choose, once nothing else changes for a moment. */
	request(request: SmartPreviewRequest): void {
		clearTimeout(this.#timer);
		this.#timer = setTimeout(() => {
			void this.#send(request);
		}, this.#delay);
	}

	/** Asks at once: the editor's first look. */
	now(request: SmartPreviewRequest): Promise<void> {
		clearTimeout(this.#timer);
		return this.#send(request);
	}

	async #send(request: SmartPreviewRequest): Promise<void> {
		this.#controller?.abort();
		const controller = new AbortController();
		this.#controller = controller;
		try {
			const answer = await this.#ask(request, controller.signal);
			if (this.#controller === controller) {
				this.answer = answer;
				this.failed = false;
			}
		} catch {
			if (this.#controller === controller && !controller.signal.aborted) {
				this.failed = true;
			}
		}
	}

	/** Stops waiting and asking: the editor closed. */
	stop(): void {
		clearTimeout(this.#timer);
		this.#controller?.abort();
		this.#controller = null;
	}
}
