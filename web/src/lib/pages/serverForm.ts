import { plural } from '#lib/library/home.ts';
import type {
	FolderKind,
	FolderListing,
	LibraryFacts,
	LogLevel,
	ScanResult,
	SecretSetting,
	ServerSettings,
	Setting,
	SettingKey,
	SettingsChange
} from '#lib/library/source.ts';

/** The Server page's sections, in the order they're shown. */
export type SectionId = 'library' | 'lastfm' | 'scanning' | 'advanced';

export const sections: readonly SectionId[] = ['library', 'lastfm', 'scanning', 'advanced'];

/** What happens to the shared secret on save. */
export type SecretDraft = { kind: 'keep' } | { kind: 'replace'; text: string } | { kind: 'clear' };

/** What the page's fields hold, as they're edited. */
export interface Draft {
	roots: string[];
	tagWrites: boolean;
	apiKey: string;
	secret: SecretDraft;
	quickScan: string;
	/** The grace in days, as typed. */
	graceDays: string;
	baseUrl: string;
	/** The proxies as typed: commas between them. */
	proxies: string;
	level: LogLevel;
}

/** Each section's fields. */
const fieldsOf: Record<SectionId, readonly (keyof Draft)[]> = {
	library: ['roots', 'tagWrites'],
	lastfm: ['apiKey', 'secret'],
	scanning: ['quickScan', 'graceDays'],
	advanced: ['baseUrl', 'proxies', 'level']
};

/** The section a key's field is in, for a refusal the server names. */
export const sectionOfKey: Record<SettingKey, SectionId> = {
	'library.roots': 'library',
	'library.tag_writes': 'library',
	'lastfm.api_key': 'lastfm',
	'lastfm.shared_secret': 'lastfm',
	'library.quick_scan': 'scanning',
	'library.missing_grace': 'scanning',
	'server.base_url': 'advanced',
	'server.trusted_proxies': 'advanced',
	'log.level': 'advanced'
};

const secondsPerDay = 86_400;

/** Days in a Go duration such as `720h0m0s`, or null for one this can't read. */
export function durationDays(duration: string): number | null {
	const match = /^(?:(\d+(?:\.\d+)?)h)?(?:(\d+(?:\.\d+)?)m)?(?:(\d+(?:\.\d+)?)s)?$/.exec(duration);
	if (match === null || duration === '') {
		return null;
	}
	const [, hours = '0', minutes = '0', seconds = '0'] = match;
	return (Number(hours) * 3600 + Number(minutes) * 60 + Number(seconds)) / secondsPerDay;
}

/** Days as a person types them: whole when they are, else to two places. */
export function daysText(days: number): string {
	return Number.isInteger(days) ? String(days) : String(Math.round(days * 100) / 100);
}

/** The most days the server keeps a missing song: ten years. */
export const maxGraceDays = 3650;

/** Typed days as a duration the server takes, or null for text that isn't 0 to ten years. */
export function graceOf(text: string): string | null {
	const days = Number(text.trim());
	if (text.trim() === '' || !Number.isFinite(days) || days < 0 || days > maxGraceDays) {
		return null;
	}
	return `${String(Math.round(days * secondsPerDay))}s`;
}

/** Proxies as typed, a comma between each. */
export function proxiesText(list: readonly string[]): string {
	return list.join(', ');
}

/** Typed proxies as a list, dropping empty items. */
export function proxiesOf(text: string): string[] {
	return text
		.split(',')
		.map((item) => item.trim())
		.filter((item) => item !== '');
}

/** The fields as the server has them. */
export function draftOf({ settings: s }: ServerSettings): Draft {
	const grace = durationDays(s['library.missing_grace'].value);
	return {
		roots: [...s['library.roots'].value],
		tagWrites: s['library.tag_writes'].value,
		apiKey: s['lastfm.api_key'].value,
		secret: { kind: 'keep' },
		quickScan: s['library.quick_scan'].value,
		graceDays: grace === null ? s['library.missing_grace'].value : daysText(grace),
		baseUrl: s['server.base_url'].value,
		proxies: proxiesText(s['server.trusted_proxies'].value),
		level: s['log.level'].value
	};
}

function same(a: Draft[keyof Draft], b: Draft[keyof Draft]): boolean {
	return JSON.stringify(a) === JSON.stringify(b);
}

/** Whether a section's fields differ from the server's. */
export function dirty(base: Draft, draft: Draft, section: SectionId): boolean {
	return fieldsOf[section].some((field) => !same(base[field], draft[field]));
}

/** Puts a section's fields back as the server has them. */
export function undone(base: Draft, draft: Draft, section: SectionId): Draft {
	const next = { ...draft };
	for (const field of fieldsOf[section]) {
		Object.assign(next, { [field]: structuredClone(base[field]) });
	}
	return next;
}

/**
 * The fields after a section is saved: that section takes the server's new values, and every other
 * keeps what's being typed in it, or follows the server where nothing is.
 */
export function afterSave(oldBase: Draft, newBase: Draft, draft: Draft, saved: SectionId): Draft {
	const next = { ...newBase };
	for (const section of sections) {
		if (section === saved) {
			continue;
		}
		for (const field of fieldsOf[section]) {
			if (!same(oldBase[field], draft[field])) {
				Object.assign(next, { [field]: draft[field] });
			}
		}
	}
	return next;
}

/** Why a section can't be saved as typed. */
export type DraftProblem = { key: SettingKey; message: string };

/**
 * The change that saves a section: only the keys that changed. A problem says what the server
 * would refuse anyway, before asking it.
 */
export function changeFor(
	base: Draft,
	draft: Draft,
	section: SectionId
): { change: SettingsChange } | { problem: DraftProblem } {
	const change: SettingsChange = {};
	const differs = (field: keyof Draft): boolean => !same(base[field], draft[field]);
	switch (section) {
		case 'library':
			if (differs('roots')) change['library.roots'] = draft.roots;
			if (differs('tagWrites')) change['library.tag_writes'] = draft.tagWrites;
			break;
		case 'lastfm':
			if (differs('apiKey')) change['lastfm.api_key'] = draft.apiKey.trim();
			if (draft.secret.kind === 'replace') {
				if (draft.secret.text.trim() === '') {
					return {
						problem: {
							key: 'lastfm.shared_secret',
							message: 'Paste the shared secret, or keep the one set.'
						}
					};
				}
				change['lastfm.shared_secret'] = draft.secret.text.trim();
			} else if (draft.secret.kind === 'clear') {
				change['lastfm.shared_secret'] = null;
			}
			break;
		case 'scanning':
			if (differs('quickScan')) {
				if (!/^([01]\d|2[0-3]):[0-5]\d$/.test(draft.quickScan)) {
					return { problem: { key: 'library.quick_scan', message: 'Give the time as HH:MM.' } };
				}
				change['library.quick_scan'] = draft.quickScan;
			}
			if (differs('graceDays')) {
				const grace = graceOf(draft.graceDays);
				if (grace === null) {
					return {
						problem: {
							key: 'library.missing_grace',
							message: `Give a number of days from 0 to ${String(maxGraceDays)}.`
						}
					};
				}
				change['library.missing_grace'] = grace;
			}
			break;
		case 'advanced':
			if (differs('baseUrl')) change['server.base_url'] = draft.baseUrl.trim();
			if (differs('proxies')) change['server.trusted_proxies'] = proxiesOf(draft.proxies);
			if (differs('level')) change['log.level'] = draft.level;
			break;
	}
	return { change };
}

/** Where a value came from, when that's a file, a variable or a flag: the page says so. */
export function fromLine(
	setting: Pick<Setting<unknown> | SecretSetting, 'source' | 'from'>
): string | null {
	switch (setting.source) {
		case 'server.toml':
		case 'env':
		case 'flag':
			return `FIRST SET FROM ${setting.from ?? setting.source}`;
		case 'default':
		case 'web':
	}
	return null;
}

/** The note beside a field that waits for a restart. */
export function restartNote(setting: Setting<unknown>): string | null {
	if (setting.restartRequired !== true) {
		return null;
	}
	return setting.pendingRestart === true ? 'SAVED · AFTER A RESTART' : 'AFTER A RESTART';
}

/** The log levels, as the page names them. */
export const levels: readonly { value: LogLevel; label: string }[] = [
	{ value: 'debug', label: 'Debug' },
	{ value: 'info', label: 'Info' },
	{ value: 'warn', label: 'Warnings' },
	{ value: 'error', label: 'Errors' }
];

/** Bytes as a person reads a library's size. */
export function size(bytes: number): string {
	const units = ['B', 'KB', 'MB', 'GB', 'TB'] as const;
	let value = bytes;
	let unit = 0;
	while (value >= 1000 && unit < units.length - 1) {
		value /= 1000;
		unit += 1;
	}
	const shown = value >= 100 || unit === 0 ? Math.round(value) : Math.round(value * 10) / 10;
	return `${shown.toLocaleString('en-US')} ${units.at(unit) ?? 'B'}`;
}

/** What one library folder holds, under its path. */
export function folderLine(tracks: number, bytes: number, saved: boolean): string {
	if (!saved) {
		return 'Added when you save. A scan starts then.';
	}
	return tracks === 0 ? 'Nothing read yet' : `${plural(tracks, 'song', 'songs')} · ${size(bytes)}`;
}

/** The library in one line, beside the headline. */
export function libraryFactsLine(facts: LibraryFacts, bytes: number): string {
	const parts = [plural(facts.albums, 'album', 'albums'), plural(facts.tracks, 'song', 'songs')];
	return bytes > 0 ? [...parts, size(bytes)].join(' · ') : parts.join(' · ');
}

const dayMs = 86_400_000;

/** When the last scan ended and how long it took, such as `Today 03:00 · took 4 min`. */
export function lastScanLine(last: ScanResult | undefined, running: boolean, now: number): string {
	if (running) {
		return 'Running now';
	}
	if (last === undefined) {
		return 'None since the server started';
	}
	const ended = new Date(last.endedAt);
	const startOfToday = new Date(now);
	startOfToday.setHours(0, 0, 0, 0);
	const time = ended.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' });
	let day: string;
	if (ended.getTime() >= startOfToday.getTime()) {
		day = 'Today';
	} else if (ended.getTime() >= startOfToday.getTime() - dayMs) {
		day = 'Yesterday';
	} else {
		day = ended.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
	}
	const ms = Date.parse(last.endedAt) - Date.parse(last.startedAt);
	const took =
		ms < 60_000
			? `${String(Math.max(1, Math.round(ms / 1000)))} s`
			: `${String(Math.round(ms / 60_000))} min`;
	return last.outcome === 'failed' ? `${day} ${time} · failed` : `${day} ${time} · took ${took}`;
}

/** The breadcrumb to a path: `/`, then each folder on the way. */
export function crumbsOf(path: string): { name: string; path: string }[] {
	const crumbs = [{ name: '/', path: '/' }];
	let at = '';
	for (const name of path.split('/').filter((part) => part !== '')) {
		at = `${at}/${name}`;
		crumbs.push({ name, path: at });
	}
	return crumbs;
}

/** Why a folder in the picker can't be added, beside its name. */
export function kindNote(kind: FolderKind): string {
	switch (kind) {
		case 'library':
			return 'in your library';
		case 'inside':
			return 'inside your library';
		case 'holds':
			return 'holds a library folder';
		case 'data':
			return 'Kitsune’s data';
		case 'config':
			return 'Kitsune’s config';
		case 'folder':
	}
	return '';
}

/** What the picker says of the folder it's in: whether it can be added, and what it holds. */
export function checkLine({ path, kind, audio }: FolderListing): string {
	if (path === '/') {
		return 'Open the folder your music is in.';
	}
	switch (kind) {
		case 'library':
			return `${path} is in your library already.`;
		case 'inside':
			return `${path} is inside a library folder, so its songs are in already.`;
		case 'holds':
			return `${path} holds a library folder. Remove that one first to add this instead.`;
		case 'data':
		case 'config':
			return `${path} is Kitsune’s own, not music.`;
		case 'folder':
	}
	if (audio.files === 0) {
		return audio.complete
			? `${path} is readable, but holds no audio files Kitsune reads.`
			: `${path} is readable. Counting its files took too long, so it wasn’t finished.`;
	}
	const files = `${audio.complete ? '' : 'at least '}${plural(audio.files, 'audio file', 'audio files')}`;
	return `${path} is readable: ${files} in ${plural(audio.folders, 'folder', 'folders')}. Adding it starts a scan.`;
}
