import type {
	FolderKind,
	FolderListing,
	ServerSettings,
	Setting,
	SettingsChange
} from './source.ts';

/** The defaults the server gives a key nothing has set. */
const defaults = {
	'lastfm.api_key': '',
	'library.roots': [] as string[],
	'library.tag_writes': false,
	'library.quick_scan': '03:30',
	'library.missing_grace': '24h0m0s',
	'log.level': 'info',
	'server.base_url': '',
	'server.trusted_proxies': [] as string[]
} as const;

function unset<T>(value: T, restartRequired = false): Setting<T> {
	return { value, source: 'default', ...(restartRequired ? { restartRequired } : {}) };
}

/** The boards' server: one folder from the file, a key from the environment when Last.fm is on. */
export function settingsFixture(lastfmConfigured: boolean): ServerSettings {
	return {
		settings: {
			'lastfm.api_key': lastfmConfigured
				? { value: '3f9c1e0b7a6d45e2b8c4a91f0d2e7b36', source: 'web' }
				: unset(''),
			'lastfm.shared_secret': lastfmConfigured
				? { set: true, source: 'env', from: 'KITSUNE_LASTFM_SHARED_SECRET' }
				: { set: false, source: 'default' },
			'library.roots': { value: ['/music'], source: 'server.toml', from: 'server.toml' },
			'library.tag_writes': unset(false),
			'library.quick_scan': { value: '03:00', source: 'web' },
			'library.missing_grace': { value: '720h0m0s', source: 'web' },
			'log.level': unset('info'),
			'server.base_url': unset('', true),
			'server.trusted_proxies': unset([], true)
		},
		listen: ':4533',
		listenSource: 'server.toml',
		version: '2026.11.07 · 8ece8cd',
		folders: [{ path: '/music', tracks: 7586, bytes: 140_660_000_000 }]
	};
}

/** The boards' container: what compose.yaml mounts, and Kitsune's own two. */
const tree: Record<string, { folders: string[]; files: number; holding: number }> = {
	'/': { folders: ['audiobooks', 'config', 'data', 'mnt', 'music'], files: 0, holding: 0 },
	'/audiobooks': { folders: ['Dune', 'The Hobbit'], files: 612, holding: 38 },
	'/audiobooks/Dune': { folders: [], files: 40, holding: 2 },
	'/audiobooks/The Hobbit': { folders: [], files: 22, holding: 1 },
	'/config': { folders: [], files: 0, holding: 0 },
	'/data': { folders: ['artwork'], files: 0, holding: 0 },
	'/data/artwork': { folders: [], files: 0, holding: 0 },
	'/mnt': { folders: ['usb'], files: 0, holding: 0 },
	'/mnt/usb': { folders: [], files: 3, holding: 1 },
	'/music': { folders: ['Mere Coast'], files: 7586, holding: 927 },
	'/music/Mere Coast': { folders: [], files: 12, holding: 1 }
};

function kindOf(path: string, roots: readonly string[]): FolderKind {
	const under = (parent: string, child: string): boolean =>
		child === parent || child.startsWith(parent === '/' ? '/' : `${parent}/`);
	for (const root of roots) {
		if (path === root) return 'library';
		if (under(root, path)) return 'inside';
		if (under(path, root)) return 'holds';
	}
	if (under('/data', path)) return 'data';
	return path === '/config' ? 'config' : 'folder';
}

/** Lists a folder of the boards' container, as `GET /admin/folders` would. */
export function foldersFixture(path: string, roots: readonly string[]): FolderListing | null {
	const here = tree[path];
	if (here === undefined) {
		return null;
	}
	const join = (name: string): string => (path === '/' ? `/${name}` : `${path}/${name}`);
	return {
		path,
		...(path === '/' ? {} : { parent: path.slice(0, path.lastIndexOf('/')) || '/' }),
		kind: kindOf(path, roots),
		folders: here.folders.map((name) => ({
			name,
			path: join(name),
			kind: kindOf(join(name), roots)
		})),
		audio: { files: here.files, folders: here.holding, complete: true }
	};
}

/** Applies a change as the server would, without its checks: a null or empty value clears. */
export function changedSettings(current: ServerSettings, change: SettingsChange): ServerSettings {
	const next = structuredClone(current);
	const s = next.settings;
	for (const [key, value] of Object.entries(change) as [keyof SettingsChange, unknown][]) {
		if (key === 'lastfm.shared_secret') {
			s[key] =
				value === null || value === ''
					? { set: false, source: 'default' }
					: { set: true, source: 'web' };
			continue;
		}
		const cleared = value === null || value === '' || (Array.isArray(value) && value.length === 0);
		const setting = s[key] as Setting<unknown>;
		const restart = setting.restartRequired === true;
		Object.assign(
			s,
			Object.fromEntries([
				[
					key,
					{
						value: cleared ? defaults[key] : value,
						source: cleared ? 'default' : 'web',
						...(restart ? { restartRequired: true, pendingRestart: true } : {})
					}
				]
			])
		);
	}
	next.folders = s['library.roots'].value.map(
		(path) =>
			current.folders.find((folder) => folder.path === path) ?? { path, tracks: 0, bytes: 0 }
	);
	return next;
}
