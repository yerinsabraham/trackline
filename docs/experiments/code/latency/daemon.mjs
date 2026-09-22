import net from 'node:net';
import fs from 'node:fs';
const SOCK = '/tmp/trackline-bench.sock';
try { fs.unlinkSync(SOCK); } catch {}
net.createServer((c) => {
  let buf = '';
  c.on('data', (d) => {
    buf += d;
    const i = buf.indexOf('\n');
    if (i < 0) return;
    let e; try { e = JSON.parse(buf.slice(0, i)); } catch { c.end('0\n'); return; }
    const blocked = JSON.stringify(e.tool_input ?? {}).includes('secrets.env');
    c.end(blocked ? '2 BLOCKED: secrets.env is off limits. Use config.local.json instead.\n' : '0\n');
  });
}).listen(SOCK, () => console.log('ready'));
