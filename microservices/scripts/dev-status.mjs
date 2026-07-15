import { execFileSync } from 'node:child_process';
import net from 'node:net';

const ports = [3000, 3001, 8081];

function dockerStatus() {
  try {
    const output = execFileSync(
      'docker',
      ['ps', '--filter', 'name=go-admin-kit-', '--format', 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'],
      { encoding: 'utf8' },
    ).trim();
    return output || 'No go-admin-kit Docker containers are running.';
  } catch {
    return 'Docker status unavailable.';
  }
}

function checkPort(port) {
  return new Promise((resolve) => {
    const socket = net.createConnection({ host: '127.0.0.1', port });
    const finish = (listening) => {
      socket.removeAllListeners();
      socket.destroy();
      resolve({ port, listening });
    };

    socket.setTimeout(500);
    socket.once('connect', () => finish(true));
    socket.once('timeout', () => finish(false));
    socket.once('error', () => finish(false));
  });
}

console.log(dockerStatus());
console.log('');
console.log('Local ports:');

const results = await Promise.all(ports.map(checkPort));
for (const result of results) {
  console.log(`  ${result.port}: ${result.listening ? 'listening' : 'not listening'}`);
}
