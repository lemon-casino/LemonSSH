import { mkdirSync } from 'node:fs';
import path from 'node:path';
import { spawnSync } from 'node:child_process';

mkdirSync('bin', { recursive: true });
for (const command of ['netcatty-tool', 'netcatty-mcp']) {
  const name = command + (process.platform === 'win32' ? '.exe' : '');
  const result = spawnSync('go', ['build', '-trimpath', '-o', path.join('bin', name), `./cmd/${command}`], { stdio: 'inherit' });
  if (result.error) throw result.error;
  if (result.status !== 0) process.exit(result.status ?? 1);
}
