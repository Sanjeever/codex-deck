import {readFileSync, writeFileSync} from 'node:fs';
import {fileURLToPath} from 'node:url';
import {resolve} from 'node:path';

const root = fileURLToPath(new URL('../', import.meta.url));
const packagePath = resolve(root, 'frontend/package.json');
const configPath = resolve(root, 'wails.json');
const pkg = JSON.parse(readFileSync(packagePath, 'utf8'));
const config = JSON.parse(readFileSync(configPath, 'utf8'));
const [command, value] = process.argv.slice(2);
if (command === 'bump') {
  if (!/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(value ?? '')) throw new Error('请输入正式版本，例如 1.0.0');
  pkg.version = value;
  config.info = {...config.info, productVersion: value};
  writeFileSync(packagePath, JSON.stringify(pkg, null, 2) + '\n');
  writeFileSync(configPath, JSON.stringify(config, null, 2) + '\n');
  console.log(`版本已更新为 ${value}，请检查并提交变更。`);
} else if (command === 'check') {
  if (value !== `v${pkg.version}` || config.info?.productVersion !== pkg.version) throw new Error('tag、package.json 与 wails.json 版本不一致');
} else throw new Error('用法：version.mjs bump <version> 或 check <tag>');
