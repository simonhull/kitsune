import type { ShuffleMark, ShuffleRules, ShuffleRun } from './source.ts';

/** A change to a person's shuffle rules, as one request makes it. */
export type RuleChange =
	/** Combine these songs into one run, in this order. */
	| { kind: 'run'; tracks: string[] }
	/** Undo a run: its songs stay, each on its own. */
	| { kind: 'unrun'; id: string }
	/** Leave songs out of shuffle, or let them back in. */
	| { kind: 'exclude'; tracks: string[]; on: boolean };

/** What someone has touched in the Shuffle tab, and nothing else: null is untouched. */
export interface RulesDraft {
	combine: boolean | null;
	/** The run's order, as dragged. */
	order: string[] | null;
	exclude: boolean | null;
}

export function emptyRulesDraft(): RulesDraft {
	return { combine: null, order: null, exclude: null };
}

export const noRules: ShuffleRules = { runs: [], excluded: [] };

function sameList(a: readonly string[], b: readonly string[]): boolean {
	return a.length === b.length && a.every((item, index) => item === b[index]);
}

/** Moves the item at `from` to `to`, as a drag or Alt with an arrow does. */
export function moveIn<T>(list: readonly T[], from: number, to: number): T[] {
	if (from < 0 || from >= list.length || to < 0 || to >= list.length || from === to) {
		return [...list];
	}
	const out = [...list];
	const [moved] = out.splice(from, 1);
	if (moved !== undefined) {
		out.splice(to, 0, moved);
	}
	return out;
}

/** The run the songs make already, when they're exactly one of the person's runs. */
export function runOf(songs: readonly string[], rules: ShuffleRules): ShuffleRun | undefined {
	return rules.runs.find(
		({ tracks }) => tracks.length === songs.length && tracks.every((id) => songs.includes(id))
	);
}

/** How many of the songs are combined with other songs, in a run they don't make up. */
export function combinedElsewhere(songs: readonly string[], rules: ShuffleRules): number {
	const own = runOf(songs, rules);
	return songs.filter((id) => rules.runs.some((run) => run !== own && run.tracks.includes(id)))
		.length;
}

export function excludedAmong(songs: readonly string[], rules: ShuffleRules): number {
	return songs.filter((id) => rules.excluded.includes(id)).length;
}

/** Whether the Combine switch is on: as touched, or as the songs stand. */
export function combineOn(
	songs: readonly string[],
	rules: ShuffleRules,
	draft: RulesDraft
): boolean {
	return songs.length >= 2 && (draft.combine ?? runOf(songs, rules) !== undefined);
}

/** The run's order: as dragged, or the run's own, or the order the songs were picked in. */
export function orderOf(
	songs: readonly string[],
	rules: ShuffleRules,
	draft: RulesDraft
): string[] {
	const held = draft.order?.filter((id) => songs.includes(id));
	if (held !== undefined && held.length === songs.length) {
		return held;
	}
	return runOf(songs, rules)?.tracks ?? [...songs];
}

/** Whether the "Exclude all" switch is on: as touched, or whether every one is left out now. */
export function excludeOn(
	songs: readonly string[],
	rules: ShuffleRules,
	draft: RulesDraft
): boolean {
	return draft.exclude ?? (songs.length > 0 && excludedAmong(songs, rules) === songs.length);
}

/** What saving the Shuffle tab sends, in order. Nothing touched sends nothing. */
export function ruleChanges(
	songs: readonly string[],
	rules: ShuffleRules,
	draft: RulesDraft
): RuleChange[] {
	const changes: RuleChange[] = [];
	const own = runOf(songs, rules);
	if (draft.combine !== null || draft.order !== null) {
		const order = orderOf(songs, rules, draft);
		if (combineOn(songs, rules, draft)) {
			if (own === undefined || !sameList(own.tracks, order)) {
				changes.push({ kind: 'run', tracks: order });
			}
		} else if (own !== undefined) {
			changes.push({ kind: 'unrun', id: own.id });
		}
	}
	if (draft.exclude !== null) {
		const tracks = songs.filter((id) => rules.excluded.includes(id) !== draft.exclude);
		if (tracks.length > 0) {
			changes.push({ kind: 'exclude', tracks, on: draft.exclude });
		}
	}
	return changes;
}

/** What the "Exclude all" switch says of the songs now. */
export function excludedNote(songs: readonly string[], rules: ShuffleRules): string {
	const count = excludedAmong(songs, rules);
	if (count === 0) {
		return songs.length === 1 ? "It isn't excluded now." : 'None of them is excluded now.';
	}
	if (count === songs.length) {
		return songs.length === 1 ? "It's excluded now." : 'All of them are excluded now.';
	}
	return `${String(count)} of them ${count === 1 ? 'is' : 'are'} excluded now.`;
}

/**
 * Whether a row's run goes on into the rows above and below it, in its own order, so one
 * bracket joins them. Songs of a run on other albums, or out of order, aren't joined.
 */
export function runJoins(
	tracks: readonly { shuffle?: ShuffleMark | undefined }[],
	index: number
): { up: boolean; down: boolean } {
	const mark = tracks[index]?.shuffle;
	const joined = (step: number): boolean => {
		const other = tracks[index + step]?.shuffle;
		return (
			mark?.run !== undefined &&
			other?.run === mark.run &&
			other.position === (mark.position ?? 0) + step
		);
	};
	return { up: joined(-1), down: joined(1) };
}
