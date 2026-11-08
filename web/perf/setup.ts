/**
 * Playwright's global setup: writes each library (once; later runs reuse it), starts a release
 * server on a fresh database for each, waits for its first scan, and hands back the teardown, which
 * stops them and writes the report.
 */
import { execFileSync, spawn, type ChildProcess } from 'node:child_process';
import { existsSync, mkdirSync, openSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { writeLibrary } from './library.ts';
import { writeReport } from './report.ts';
import { libraries, libraryDir, perfDir, resultsDir, serverBinary, user } from './servers.ts';

const readyTimeoutMs = 10 * 60_000;

async function ready(url: string, server: ChildProcess, log: string): Promise<void> {
	const deadline = Date.now() + readyTimeoutMs;
	while (Date.now() < deadline) {
		if (server.exitCode !== null) {
			throw new Error(`the server stopped with status ${String(server.exitCode)}; see ${log}`);
		}
		try {
			const response = await fetch(`${url}/api/v1/readyz`);
			if (response.status === 204) {
				return;
			}
		} catch {
			// Not listening yet.
		}
		await new Promise((done) => setTimeout(done, 250));
	}
	throw new Error(`not ready after ${String(readyTimeoutMs)} ms; see ${log}`);
}

export default async function setup(): Promise<() => Promise<void>> {
	if (!existsSync(serverBinary)) {
		throw new Error(
			`no server at ${serverBinary}: run scripts/perf.sh, or set KITSUNE_PERF_SERVER`
		);
	}
	rmSync(resultsDir, { recursive: true, force: true });
	mkdirSync(resultsDir, { recursive: true });
	const servers: ChildProcess[] = [];
	// Nothing from the caller's environment seeds a setting.
	const env = Object.fromEntries(
		Object.entries(process.env).filter(([key]) => !key.startsWith('KITSUNE_'))
	);
	for (const library of libraries) {
		const dir = libraryDir(library);
		const music = join(dir, 'music');
		const started = performance.now();
		const manifest = writeLibrary(music, join(perfDir, 'templates'), library);
		const wrote = performance.now();
		const data = join(dir, 'data');
		rmSync(data, { recursive: true, force: true });
		const config = join(dir, 'server.toml');
		writeFileSync(
			config,
			[
				'[server]',
				`listen = "127.0.0.1:${String(library.port)}"`,
				'[library]',
				`roots = [${JSON.stringify(music)}]`,
				'[storage]',
				`data_dir = ${JSON.stringify(data)}`,
				'[log]',
				'level = "warn"'
			].join('\n')
		);
		execFileSync(
			serverBinary,
			['users', 'add', '-admin', '-password-stdin', '-config', config, user.name],
			{ input: `${user.password}\n`, env, stdio: ['pipe', 'ignore', 'inherit'] }
		);
		const log = join(dir, 'server.log');
		const out = openSync(log, 'w');
		const server = spawn(serverBinary, ['serve', '-config', config], {
			env,
			stdio: ['ignore', out, out]
		});
		servers.push(server);
		await ready(`http://127.0.0.1:${String(library.port)}`, server, log);
		console.log(
			`perf: ${library.name}: ${String(manifest.songs)} songs, ${String(manifest.albums)} albums, ` +
				`${String(manifest.artists)} artists; library ${(wrote - started).toFixed(0)} ms, ` +
				`first scan ${(performance.now() - wrote).toFixed(0)} ms`
		);
	}
	return async () => {
		for (const server of servers) {
			server.kill('SIGTERM');
		}
		await Promise.all(
			servers.map(
				(server) =>
					new Promise((done) => {
						if (server.exitCode !== null) {
							done(null);
						} else {
							server.once('exit', done);
						}
					})
			)
		);
		writeReport();
	};
}
