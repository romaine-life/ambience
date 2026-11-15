import fs from 'node:fs';
import path from 'node:path';
import {execFileSync} from 'node:child_process';

for (const dir of ['cmd/ambience/web', 'scripts']) {
  for (const file of fs.readdirSync(dir, {recursive: true})) {
    if (/\.(?:js|mjs)$/.test(file)) {
      execFileSync(process.execPath, ['--check', path.join(dir, file)], {stdio: 'inherit'});
    }
  }
}
