import { mkdirSync, rmSync } from 'node:fs';
import path from 'node:path';
import { spawnSync } from 'node:child_process';

mkdirSync('bin', { recursive: true });
const nativeTools = [
  { command: 'netcatty-tool', output: 'LemonSSH-tool' },
  { command: 'netcatty-mcp', output: 'LemonSSH-mcp' },
];
for (const legacy of ['netcatty-tool', 'netcatty-mcp']) {
  rmSync(path.join('bin', legacy + (process.platform === 'win32' ? '.exe' : '')), { force: true });
}
for (const { command, output } of nativeTools) {
  const name = output + (process.platform === 'win32' ? '.exe' : '');
  const result = spawnSync('go', ['build', '-trimpath', '-o', path.join('bin', name), `./cmd/${command}`], { stdio: 'inherit' });
  if (result.error) throw result.error;
  if (result.status !== 0) process.exit(result.status ?? 1);
}
