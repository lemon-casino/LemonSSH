import { mkdirSync, rmSync } from 'node:fs';
import path from 'node:path';
import { spawnSync } from 'node:child_process';

mkdirSync('bin', { recursive: true });
const nativeTools = [
  { command: 'lemonssh-tool', output: 'LemonSSH-tool' },
  { command: 'lemonssh-mcp', output: 'LemonSSH-mcp' },
];
// Legacy cleanup list must keep the OLD names: it removes stale
// netcatty-tool.exe / netcatty-mcp.exe left by the previous release so the
// renamed binaries are not shadowed. Renaming these entries would leak the
// legacy executables into later packaging steps.
for (const legacy of ['netcatty-tool', 'netcatty-mcp']) {
  rmSync(path.join('bin', legacy + (process.platform === 'win32' ? '.exe' : '')), { force: true });
}
for (const { command, output } of nativeTools) {
  const name = output + (process.platform === 'win32' ? '.exe' : '');
  const result = spawnSync('go', ['build', '-trimpath', '-o', path.join('bin', name), `./cmd/${command}`], { stdio: 'inherit' });
  if (result.error) throw result.error;
  if (result.status !== 0) process.exit(result.status ?? 1);
}
