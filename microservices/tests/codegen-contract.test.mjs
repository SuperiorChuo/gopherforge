import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { cp, mkdtemp, readFile, rm, symlink } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const repositoryRoot = fileURLToPath(new URL('../..', import.meta.url));
const systemServiceRoot = path.join(repositoryRoot, 'microservices/services/system');

test('代码生成产物可接入并编译', { timeout: 180_000 }, async () => {
  const result = await runCodegenFixture();

  assert.equal(result.fixtureExitCode, 0, result.fixtureOutput);
  assert.equal(result.goTestExitCode, 0, result.goTestOutput);
  assert.equal(result.webBuildExitCode, 0, result.webBuildOutput);
  assert.deepEqual(result.unresolvedMarkers, []);
});

test('system-service 镜像提供只读预检快照且默认关闭直写', async () => {
  const dockerfile = await readFile(
    path.join(repositoryRoot, 'microservices/services/system/Dockerfile'),
    'utf8',
  );
  const compose = await readFile(path.join(repositoryRoot, 'microservices/docker-compose.yml'), 'utf8');
  const stack = await readFile(path.join(repositoryRoot, 'microservices/docker-stack.yml'), 'utf8');

  for (const source of [
    'services/system/internal/api/routes.go',
    'web/src/router/index.tsx',
    'web/src/layouts/menu-defs.tsx',
    'services/system/internal/service/system/menu_seed.go',
    'services/monitor/migrations/',
  ]) {
    assert.ok(dockerfile.includes(`COPY ${source}`), `镜像快照缺少 ${source}`);
  }
  assert.match(dockerfile, /COPY --from=snapshot \/repository-snapshot \/app\/repository-snapshot/);
  assert.match(compose, /CODEGEN_REPO_ROOT: \$\{CODEGEN_REPO_ROOT:-\/app\/repository-snapshot\}/);
  assert.match(compose, /CODEGEN_WRITE_ENABLED: \$\{CODEGEN_WRITE_ENABLED:-false\}/);
  assert.match(stack, /CODEGEN_REPO_ROOT: \$\{CODEGEN_REPO_ROOT:-\/app\/repository-snapshot\}/);
  assert.match(stack, /CODEGEN_WRITE_ENABLED: \$\{CODEGEN_WRITE_ENABLED:-false\}/);
});

test('静态验收入口将同源 API 代理到 Traefik 网关', async () => {
  const nginx = await readFile(path.join(repositoryRoot, 'microservices/web/nginx.conf'), 'utf8');
  const apiLocation = nginx.match(/location \/api\/ \{([\s\S]*?)^\s*\}/m)?.[1];

  assert.ok(apiLocation, '缺少 /api/ 代理 location');
  const proxyTarget = apiLocation.match(/proxy_pass\s+([^;]+);/)?.[1];
  assert.ok(
    proxyTarget === 'http://go-admin-kit-gateway' || proxyTarget === '$gateway_upstream',
    `/api/ 未代理到 Traefik 网关，当前目标为 ${proxyTarget || '空'}`,
  );
  if (proxyTarget === '$gateway_upstream') {
    assert.match(nginx, /set \$gateway_upstream http:\/\/go-admin-kit-gateway(?::80)?;/);
  }
  assert.match(nginx, /proxy_set_header X-Forwarded-Proto \$scheme;/);
  assert.match(apiLocation, /proxy_read_timeout 120s;/);
  assert.match(apiLocation, /proxy_buffering off;/);
});

test('代码生成长操作覆盖全局短请求超时', async () => {
  const codegenAPI = await readFile(
    path.join(repositoryRoot, 'microservices/web/src/api/system/codegen.ts'),
    'utf8',
  );

  assert.match(codegenAPI, /const CODEGEN_REQUEST_TIMEOUT = 90_000/);
  assert.match(
    codegenAPI,
    /request\.post\('\/api\/v1\/codegen\/preview', codegenRequest, \{\s*timeout: CODEGEN_REQUEST_TIMEOUT,?\s*\}\)/,
    'preview 未覆盖全局 15 秒请求超时',
  );
  assert.match(
    codegenAPI,
    /request\.post\('\/api\/v1\/codegen\/download', \{[\s\S]*?expected_digest: expectedDigest,[\s\S]*?\}, \{[^}]*timeout: CODEGEN_REQUEST_TIMEOUT,?[^}]*\}\)/,
    'download 未覆盖全局 15 秒请求超时',
  );
  assert.match(
    codegenAPI,
    /request\.post\('\/api\/v1\/codegen\/write', \{[\s\S]*?confirmation,[\s\S]*?\}, \{[^}]*timeout: CODEGEN_REQUEST_TIMEOUT,?[^}]*\}\)/,
    'write 未覆盖全局 15 秒请求超时',
  );
});

async function runCodegenFixture() {
  const temporaryRoot = await mkdtemp(path.join(tmpdir(), 'go-admin-kit-codegen-'));
  try {
    await copyRepository(temporaryRoot);
    await linkWebDependencies(temporaryRoot);

    const fixture = await runCommand(
      'go',
      ['run', './cmd/codegen-fixture', '--repo', temporaryRoot],
      systemServiceRoot,
    );
    if (fixture.exitCode !== 0) {
      return failedResult('fixture', fixture);
    }

    const manifest = JSON.parse(fixture.stdout.trim());
    const temporarySystemRoot = path.join(temporaryRoot, 'microservices/services/system');
    const temporaryWebRoot = path.join(temporaryRoot, 'microservices/web');
    const goTest = await runCommand('go', ['test', '-run', '^$', './...'], temporarySystemRoot);
    const webBuild = await runCommand('npm', ['run', 'build'], temporaryWebRoot);
    const unresolvedMarkers = await scanUnresolvedMarkers(temporaryRoot, manifest.paths);

    return {
      fixtureExitCode: fixture.exitCode,
      fixtureOutput: commandOutput(fixture),
      goTestExitCode: goTest.exitCode,
      goTestOutput: commandOutput(goTest),
      webBuildExitCode: webBuild.exitCode,
      webBuildOutput: commandOutput(webBuild),
      unresolvedMarkers,
    };
  } finally {
    await rm(temporaryRoot, { recursive: true, force: true });
  }
}

async function copyRepository(destination) {
  const excludedNames = new Set([
    '.codegraph',
    '.git',
    '.worktrees',
    'dist',
    'node_modules',
    'test-results',
  ]);
  await cp(repositoryRoot, destination, {
    recursive: true,
    filter: (source) => source === repositoryRoot || !excludedNames.has(path.basename(source)),
  });
}

async function linkWebDependencies(temporaryRoot) {
  const source = path.join(repositoryRoot, 'microservices/web/node_modules');
  const destination = path.join(temporaryRoot, 'microservices/web/node_modules');
  await symlink(source, destination, 'dir');
}

async function scanUnresolvedMarkers(root, paths) {
  const markers = ['[[', ']]', '{{.', 'TODO_CODEGEN', '手动接入'];
  const unresolved = [];
  for (const relative of paths) {
    const content = await readFile(path.join(root, relative), 'utf8');
    for (const marker of markers) {
      if (content.includes(marker)) unresolved.push(`${relative}: ${marker}`);
    }
  }
  return unresolved;
}

function runCommand(command, args, cwd) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, { cwd, env: process.env, stdio: ['ignore', 'pipe', 'pipe'] });
    let stdout = '';
    let stderr = '';
    child.stdout.setEncoding('utf8');
    child.stderr.setEncoding('utf8');
    child.stdout.on('data', (chunk) => { stdout += chunk; });
    child.stderr.on('data', (chunk) => { stderr += chunk; });
    child.on('error', reject);
    child.on('close', (exitCode) => resolve({ exitCode, stdout, stderr }));
  });
}

function failedResult(stage, command) {
  const output = commandOutput(command);
  return {
    fixtureExitCode: stage === 'fixture' ? command.exitCode : 0,
    fixtureOutput: output,
    goTestExitCode: -1,
    goTestOutput: output,
    webBuildExitCode: -1,
    webBuildOutput: output,
    unresolvedMarkers: [],
  };
}

function commandOutput(command) {
  return [command.stdout, command.stderr].filter(Boolean).join('\n');
}
