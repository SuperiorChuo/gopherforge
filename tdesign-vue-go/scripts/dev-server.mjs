import { spawn, spawnSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const scriptPath = fileURLToPath(import.meta.url);
const scriptDir = path.dirname(scriptPath);
const appRoot = path.resolve(scriptDir, '..');
const viteBin = path.join(appRoot, 'node_modules', 'vite', 'bin', 'vite.js');
const normalizedAppRoot = normalizePath(appRoot);
const DEFAULT_DEV_SERVER_PORT = 3002;
const args = process.argv.slice(2);
const guardOnly = args.includes('--guard-only');
const rawViteArgs = args.filter((arg) => arg !== '--guard-only');
const port = resolvePort(rawViteArgs);
const viteArgs = withStrictPort(rawViteArgs, port);

function normalizePath(value) {
  return path.resolve(value).replace(/\\/g, '/').toLowerCase();
}

function normalizeText(value) {
  return String(value || '').replace(/\\/g, '/').toLowerCase();
}

function fail(message) {
  console.error(`[dev-server] ${message}`);
  process.exit(1);
}

function info(message) {
  console.log(`[dev-server] ${message}`);
}

function resolvePort(commandArgs) {
  for (let index = 0; index < commandArgs.length; index += 1) {
    const arg = commandArgs[index];

    if ((arg === '--port' || arg === '-p') && commandArgs[index + 1]) {
      return Number(commandArgs[index + 1]);
    }

    if (arg.startsWith('--port=')) {
      return Number(arg.slice('--port='.length));
    }
  }

  const envPort = process.env.PORT || process.env.VITE_PORT;
  return envPort ? Number(envPort) : DEFAULT_DEV_SERVER_PORT;
}

function withStrictPort(commandArgs, targetPort) {
  if (!Number.isFinite(targetPort) || commandArgs.some((arg) => arg === '--strictPort' || arg.startsWith('--strictPort='))) {
    return commandArgs;
  }

  return [...commandArgs, '--strictPort'];
}

function isSuperpowersWorktree(text) {
  return normalizeText(text).includes('/.config/superpowers/worktrees/');
}

function isSafeStaleTdesignVite(commandLine) {
  const command = normalizeText(commandLine);

  return (
    isSuperpowersWorktree(command) &&
    command.includes('/tdesign-vue-go/') &&
    command.includes('/vite/')
  );
}

function getWindowsPortListeners(targetPort) {
  if (process.platform !== 'win32' || !Number.isFinite(targetPort)) {
    return [];
  }

  const command = `
$connections = @(Get-NetTCPConnection -State Listen -LocalPort ${targetPort} -ErrorAction SilentlyContinue)
$items = foreach ($connection in $connections) {
  $proc = Get-CimInstance Win32_Process -Filter "ProcessId = $($connection.OwningProcess)" -ErrorAction SilentlyContinue
  [pscustomobject]@{
    processId = [int]$connection.OwningProcess
    parentProcessId = if ($proc) { [int]$proc.ParentProcessId } else { 0 }
    commandLine = if ($proc) { [string]$proc.CommandLine } else { "" }
  }
}
$items | ConvertTo-Json -Compress
`;

  const result = spawnSync(
    'powershell.exe',
    ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-Command', command],
    { encoding: 'utf8' },
  );

  if (result.status !== 0) {
    fail(`Unable to inspect port ${targetPort}: ${result.stderr || result.stdout}`);
  }

  const output = result.stdout.trim();
  if (!output) {
    return [];
  }

  try {
    const parsed = JSON.parse(output);
    return Array.isArray(parsed) ? parsed : [parsed];
  } catch (error) {
    fail(`Unable to parse port ${targetPort} listener data: ${error.message}`);
  }

  return [];
}

function stopWindowsProcess(processId) {
  const result = spawnSync(
    'powershell.exe',
    ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-Command', `Stop-Process -Id ${processId} -Force`],
    { encoding: 'utf8' },
  );

  if (result.status !== 0) {
    fail(`Unable to stop stale worktree dev server PID ${processId}: ${result.stderr || result.stdout}`);
  }
}

function sleep(ms) {
  return new Promise((resolve) => {
    setTimeout(resolve, ms);
  });
}

async function waitForPortToClear(targetPort) {
  for (let attempt = 0; attempt < 20; attempt += 1) {
    if (getWindowsPortListeners(targetPort).length === 0) {
      return;
    }

    await sleep(250);
  }

  fail(`Port ${targetPort} is still occupied after stopping stale worktree process.`);
}

async function guardWorkspaceAndPort() {
  if (!Number.isFinite(port) || port <= 0) {
    fail(`Invalid dev server port: ${port}`);
  }

  if (isSuperpowersWorktree(appRoot) && process.env.BLACK8_ALLOW_WORKTREE_DEV !== '1') {
    fail(
      [
        `Refusing to start frontend from hidden worktree: ${appRoot}`,
        'Use C:\\Users\\Administrator\\Desktop\\project\\go-admin-kit\\tdesign-vue-go instead.',
        'Set BLACK8_ALLOW_WORKTREE_DEV=1 only when this worktree is intentionally being tested.',
      ].join('\n[dev-server] '),
    );
  }

  const listeners = getWindowsPortListeners(port);
  for (const listener of listeners) {
    const processId = Number(listener.processId);
    const commandLine = String(listener.commandLine || '');
    const normalizedCommand = normalizeText(commandLine);

    if (normalizedCommand.includes(normalizedAppRoot)) {
      info(`Port ${port} is already served by this workspace (PID ${processId}).`);
      info(`Open http://127.0.0.1:${port}/ or stop the existing dev server before starting another one.`);
      process.exit(0);
    }

    if (isSafeStaleTdesignVite(commandLine) && process.env.BLACK8_AUTO_STOP_WORKTREE_DEV !== '0') {
      info(`Stopping stale hidden worktree dev server on port ${port} (PID ${processId}).`);
      stopWindowsProcess(processId);
      await waitForPortToClear(port);
      continue;
    }

    fail(
      [
        `Port ${port} is occupied by another process (PID ${processId}).`,
        commandLine,
        'Stop it first, or choose another explicit --port. Vite auto-port fallback is blocked to avoid opening the wrong workspace.',
      ].join('\n[dev-server] '),
    );
  }
}

await guardWorkspaceAndPort();

if (guardOnly) {
  info('Workspace and port guard passed.');
  process.exit(0);
}

if (!fs.existsSync(viteBin)) {
  fail(`Missing Vite binary: ${viteBin}. Run npm install first.`);
}

const child = spawn(process.execPath, [viteBin, ...viteArgs], {
  cwd: appRoot,
  env: process.env,
  stdio: 'inherit',
});

child.on('exit', (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal);
    return;
  }

  process.exit(code ?? 1);
});
