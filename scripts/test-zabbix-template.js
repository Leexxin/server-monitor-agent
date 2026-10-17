#!/usr/bin/env node
// Offline checks only: does not replace a real Zabbix configuration.import test.
'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const root = path.resolve(__dirname, '..');
const filename = path.join(root, 'docs/zabbix/template_sma_http_7.0.json');
const exported = JSON.parse(fs.readFileSync(filename, 'utf8')).zabbix_export;
assert.equal(exported.version, '7.0');
assert.equal(exported.templates.length, 1);
const template = exported.templates[0];
assert.equal(template.template, 'SMA by HTTP');
const seen = new Set();
(function checkIds(v) {
  if (Array.isArray(v)) return v.forEach(checkIds);
  if (!v || typeof v !== 'object') return;
  if (v.uuid) {
    assert.match(v.uuid, /^[a-f0-9]{12}4[a-f0-9]{3}[89ab][a-f0-9]{15}$/);
    assert(!seen.has(v.uuid), 'UUID must be unique: ' + v.uuid);
    seen.add(v.uuid);
  }
  Object.values(v).forEach(checkIds);
})(exported);
const allItems = [...template.items, ...template.discovery_rules.flatMap(r => r.item_prototypes)];
assert.equal(new Set(allItems.map(i => i.key)).size, allItems.length);
const items = Object.fromEntries(template.items.map(i => [i.key, i]));
const master = items['sma.snapshot'];
assert.equal(master.type, 'HTTP_AGENT');
assert.equal(master.request_method, 'GET');
assert.equal(master.status_codes, '200');
assert.equal(master.follow_redirects, 'NO');
assert.equal(master.url, '{$SMA.URL}/v1/snapshot');
assert(master.headers.some(h => h.name === 'Authorization' && h.value === 'Bearer {$SMA.TOKEN}'));
for (const i of [...allItems, ...template.discovery_rules]) {
  if (i.type === 'DEPENDENT') {
    assert(items[i.master_item.key], 'Missing master item');
    assert.equal(i.delay, '0');
  }
}
const macros = Object.fromEntries(template.macros.map(m => [m.macro, m]));
assert.equal(macros['{$SMA.TOKEN}'].type, 'SECRET_TEXT');
assert.equal(macros['{$SMA.TOKEN}'].value, '');
assert.equal(macros['{$SMA.URL}'].value, '');
assert(Number(macros['{$SMA.MEM.RECOVER}'].value) < Number(macros['{$SMA.MEM.WARN}'].value));
assert(Number(macros['{$SMA.FS.RECOVER}'].value) < Number(macros['{$SMA.FS.WARN}'].value));
function run(step, input) {
  assert.equal(step.type, 'JAVASCRIPT');
  return new Function('value', step.parameters[0])(typeof input === 'string' ? input : JSON.stringify(input));
}
const sample = { schemaVersion: 'v1', host: {hostname: 'test-node', os: 'linux', systemMountpoint: '/'},
  cpu: {logicalCount: 2, seconds: {cpu0: {idle: 100, user: 20}, cpu1: {idle: 100, user: 30}}},
  memory: {totalBytes: 1000, availableBytes: 375},
  filesystems: [{mountpoint: '/data', filesystemType: 'xfs', sizeBytes: 2000, availableBytes: 1000},
    {mountpoint: '/', filesystemType: 'ext4', sizeBytes: 1000, availableBytes: 200}],
  disks: [{device: 'vda', readBytes: 1000}], errors: [] };
assert.equal(run(master.preprocessing[0], sample), JSON.stringify(sample));
assert.throws(() => run(master.preprocessing[0], {...sample, schemaVersion: 'v2'}));
const mem = items['sma.memory.used.pct'].preprocessing[0];
assert.equal(run(mem, sample), 62.5);
assert.equal(run(mem, {...sample, memory: {totalBytes: 1000, availableBytes: 1000}}), 0);
for (const memory of [null, {totalBytes: 0, availableBytes: 0}, {totalBytes: 1000, availableBytes: 1001}, {totalBytes: 1000, availableBytes: -1}]) {
  assert.throws(() => run(mem, {...sample, memory}));
}
const system = items['sma.fs.system.used.pct'].preprocessing[0];
assert.equal(run(system, sample), 80); // Not the first array element.
assert.equal(run(system, {...sample, host: {hostname: 'old-linux'}}), 80);
const windows = {...sample, host: {hostname: 'test-win', os: 'windows', systemMountpoint: 'C:\\'},
  cpu: {logicalCount: 2, seconds: {total: {idle: 200, user: 50, system: 20}}},
  filesystems: [{mountpoint: 'D:\\', sizeBytes: 2000, availableBytes: 1000},
    {mountpoint: 'C:\\', sizeBytes: 1000, availableBytes: 200}]};
assert.equal(run(system, windows), 80);
assert.throws(() => run(system, {...windows, host: {os: 'windows'}}));
assert.throws(() => run(system, {...sample, filesystems: []}));
const cpu = items['sma.cpu.used.pct'].preprocessing;
assert.equal(cpu[1].type, 'CHANGE_PER_SECOND');
const baseline = run(cpu[0], sample);
assert.equal(baseline, 100);
assert.equal(run(cpu[0], windows), 100);
const next = JSON.parse(JSON.stringify(sample));
next.cpu.seconds.cpu0.idle += 24;
next.cpu.seconds.cpu1.idle += 24;
const rate = (run(cpu[0], next) - baseline) / 30;
assert(Math.abs(run(cpu[2], String(rate)) - 20) < 1e-10);
assert.equal(run(cpu[2], '1'), 0);
assert.equal(run(cpu[2], '0'), 100);
assert.throws(() => run(cpu[2], '1.1'));
assert.throws(() => run(cpu[0], {...sample, cpu: {logicalCount: 0}}));
assert.throws(() => run(cpu[0], {...sample, cpu: {logicalCount: 3, seconds: sample.cpu.seconds}}));
for (const rule of template.discovery_rules) {
  const field = rule.key === 'sma.fs.discovery' ? 'filesystems' : 'disks';
  const collector = field === 'filesystems' ? 'filesystem' : 'disk';
  assert.deepEqual(JSON.parse(run(rule.preprocessing[0], sample)), sample[field]);
  assert.deepEqual(JSON.parse(run(rule.preprocessing[0], {...sample, [field]: []})), []);
  assert.throws(() => run(rule.preprocessing[0], {...sample, errors: [{collector, code: 'timeout'}]}));
  assert.equal(rule.lifetime, '7d');
  assert.equal(rule.enabled_lifetime_type, 'DISABLE_AFTER');
  assert.equal(rule.enabled_lifetime, '1d');
}
const fsRule = template.discovery_rules.find(r => r.key === 'sma.fs.discovery');
const used = fsRule.item_prototypes.find(i => i.key.startsWith('sma.fs.used.pct'));
assert.equal(run(used.preprocessing[1], sample.filesystems[1]), 80);
assert.throws(() => run(used.preprocessing[1], {sizeBytes: 0, availableBytes: 0}));
const excluded = new RegExp(macros['{$SMA.FS.FSTYPE.NOT_MATCHES}'].value);
assert(excluded.test('tmpfs'));
assert(excluded.test('overlay'));
assert(!excluded.test('ext4'));
assert(!excluded.test('NTFS'));
for (const i of allItems) {
  const changes = i.preprocessing.filter(p => ['SIMPLE_CHANGE', 'CHANGE_PER_SECOND'].includes(p.type));
  assert(changes.length <= 1);
  for (const p of i.preprocessing) {
    assert(['JAVASCRIPT', 'JSONPATH', 'CHANGE_PER_SECOND'].includes(p.type));
    assert.equal(p.parameters.length, 1);
    if (p.error_handler) assert.equal(p.error_handler, 'DISCARD_VALUE');
  }
}
const n = [...template.items.flatMap(i => i.triggers || [])];
assert.equal(n.length, 5);
assert(n.every(t => t.expression.includes('/SMA by HTTP/')));
// All delivery formats must contain the same import artifact.
for (const f of ['packaging/nfpm.yaml', 'scripts/build-packages.sh', 'packaging/universal/install.sh',
  'scripts/test-install-linux.sh', 'scripts/test-universal-linux.sh']) {
  assert(fs.readFileSync(path.join(root, f), 'utf8').includes('zabbix/template_sma_http_7.0.json'), f);
}
console.log('PASS: template structure, UUIDs, references, credentials, Linux/Windows preprocessing, discovery failures and package delivery references');
