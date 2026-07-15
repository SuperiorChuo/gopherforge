import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

const logs = [
  { label: 'backend', name: 'go-admin-kit-backend.log', lines: 80 },
  { label: 'frontend', name: 'go-admin-kit-frontend-real.log', lines: 40 },
];

function candidatePaths(name) {
  const paths = [path.join(os.tmpdir(), name)];
  if (process.platform !== 'win32') {
    paths.push(path.posix.join('/tmp', name));
  }
  return [...new Set(paths)];
}

function findLogPath(name) {
  return candidatePaths(name).find((filePath) => fs.existsSync(filePath));
}

function tail(content, lineCount) {
  const lines = content.replace(/\r\n/g, '\n').split('\n');
  return lines.slice(-lineCount).join('\n');
}

for (const log of logs) {
  const filePath = findLogPath(log.name);
  console.log(`== ${log.label} ==`);
  if (!filePath) {
    console.log(`No ${log.name} file found in ${os.tmpdir()}.`);
    console.log('');
    continue;
  }

  const content = fs.readFileSync(filePath, 'utf8');
  console.log(tail(content, log.lines));
  console.log('');
}
