import { build } from 'esbuild';
import { compile } from '@tailwindcss/node';
import { Scanner } from '@tailwindcss/oxide';
import { mkdir, readFile, writeFile, copyFile, cp } from 'node:fs/promises';
import { consoleIcons } from './console-icons.mjs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const root = path.dirname(fileURLToPath(import.meta.url));
const standalone = process.argv.includes('--static');
const publishRoot = path.join(root, 'dist');
const out = standalone ? path.join(publishRoot, 'landing') : path.resolve(root, '../cmd/handoffproof/web/landing');
await mkdir(out, { recursive: true });
const reactBitsLicense = await readFile(path.join(root, 'src/reactbits/LICENSE.md'), 'utf8');
const phosphorLicense = await readFile(path.join(root, 'node_modules/@phosphor-icons/react/LICENSE'), 'utf8');
const fontLicense = await readFile(path.join(root, 'node_modules/@fontsource-variable/plus-jakarta-sans/LICENSE'), 'utf8');
await build({
  entryPoints: [path.join(root, 'src/main.tsx')], outfile: path.join(out, 'landing.js'),
  bundle: true, minify: true, jsx: 'automatic', target: ['es2022'],
  define: { 'process.env.NODE_ENV': '"production"', '__STATIC_LANDING__': String(standalone) }, legalComments: 'eof',
  banner: { js: '/*! React Bits components, reused within HandoffProof.\n' + reactBitsLicense + '\n*/' },
});
const css = await readFile(path.join(root, 'src/styles.css'), 'utf8');
const compiler = await compile(css, { base: root, onDependency() {} });
const scanner = new Scanner({ sources: [{ base: path.join(root, 'src'), pattern: '**/*.{tsx,css}', negated: false }] });
await writeFile(path.join(out, 'landing.css'), compiler.build(scanner.scan()));
await writeFile(path.join(out, 'index.html'), await readFile(path.join(root, 'index.html')));
await writeFile(path.join(out, 'THIRD_PARTY_NOTICES.txt'), reactBitsLicense+'\n\nPhosphor Icons\n'+phosphorLicense+'\n\nPlus Jakarta Sans\n'+fontLicense);
await writeFile(path.join(out, 'phosphor.svg'), consoleIcons());
await copyFile(path.join(root, 'node_modules/@fontsource-variable/plus-jakarta-sans/files/plus-jakarta-sans-latin-wght-normal.woff2'), path.join(out, 'jakarta-latin.woff2'));
await cp(path.join(root, 'screens'), path.join(out, 'screens'), { recursive:true });
if (standalone) {
  let html = await readFile(path.join(root, 'index.html'), 'utf8');
  html = html.replace('href="/">Open Control Room', 'href="https://youtu.be/LIAsrzvvelM">Watch demo');
  await writeFile(path.join(publishRoot, 'index.html'), html);
  await mkdir(path.join(publishRoot, 'assets'), { recursive: true });
  await cp(path.join(root, 'public/assets/illustrations'), path.join(publishRoot, 'assets/illustrations'), { recursive: true });
}
console.log(standalone ? 'Static landing built into landing/dist' : 'Landing built into cmd/handoffproof/web/landing');
