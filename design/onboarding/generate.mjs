// Run: npm install --prefix /tmp/troy-onboarding-render playwright
// NODE_PATH is not used by ESM; override PLAYWRIGHT_MODULE for another install.
import {createRequire} from 'node:module';
import {writeFile,mkdir,readFile} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
const require=createRequire(import.meta.url);
const {chromium}=require(process.env.PLAYWRIGHT_MODULE || '/tmp/troy-onboarding-render/node_modules/playwright');
const dir=fileURLToPath(new URL('.',import.meta.url));
// Pull the actual wordmark styling and doodle paths from the app.
const appCSS=await readFile(new URL('../../web/src/app.css',import.meta.url),'utf8');
const doodles=await readFile(new URL('../../web/src/Doodles.tsx',import.meta.url),'utf8');
const rule=name=>appCSS.match(new RegExp('\\.'+name+' \\{[^}]+\\}'))[0];
const paths=name=>[...doodles.split('export function '+name)[1].split('export function ')[0].matchAll(/ d="([^"]+)"/g)].map((m,i)=>`<path d="${m[1]}" ${name==='Helmet' && i===2?'class="hm-helmet-bristles"':''}/>`).join('');
const mark=`<div class="hm-wordmark">Troy<svg class="hm-wordmark-line" viewBox="0 0 200 16" preserveAspectRatio="none">${paths('Underline')}</svg><svg class="hm-hero-helmet" viewBox="0 0 64 64">${paths('Helmet')}</svg></div>`;
const cards=[
 ['week','Ask Brightspace<br><em>anything.</em>','what’s due this week?','Ask about deadlines, assignments, or course materials.'],
 ['exam','Study for<br>your next <em>exam.</em>','help me study for my midterm','Use your course materials to review and practice.'],
 ['room','Book a study room<br>in <em>Leavey.</em>','find a study room in Leavey for 3 at 2','Find a room in Leavey for your group, then ask Troy to book it.'],
 ['reminders','Get <em>reminders</em><br>before deadlines.','remind me tomorrow at 6 to work on my assignment','Set a reminder and Troy texts you when it is time to act.']
];
const css=`*{box-sizing:border-box}body{margin:0;color:#1b2233}.card{--serif:'Fraunces',Georgia,serif;--cardinal:#990000;width:1080px;height:1080px;background:#f8f3e9;display:flex;align-items:center;justify-content:center;flex-direction:column;padding:120px;gap:90px} ${rule('hm-wordmark')} ${rule('hm-wordmark-line')} ${rule('hm-hero-helmet')} .hm-hero-helmet .hm-helmet-bristles{stroke-width:1.3}svg{position:absolute;fill:none;stroke:currentColor;stroke-linecap:round;stroke-linejoin:round;overflow:visible}h1{font:500 90px/1.14 Fraunces;letter-spacing:-.025em;font-variation-settings:'opsz' 72,'SOFT' 60;text-align:center;margin:0;width:900px}em{font-style:normal;color:var(--cardinal)}`;
const font=`<link href="https://fonts.googleapis.com/css2?family=Caveat:wght@500;600&family=Fraunces:opsz,wght,SOFT,WONK@9..144,300..700,0..100,0..1&family=Inter:wght@400;500;600&display=swap" rel="stylesheet">`;
const browser=await chromium.launch({channel:'chrome',headless:true});
const page=await browser.newPage({viewport:{width:1080,height:1080},deviceScaleFactor:1});
await mkdir(dir+'png',{recursive:true});
for(const [i,[slug,title]] of cards.entries()){
 const html=`<!doctype html><meta charset="utf-8">${font}<style>${css}</style><article class="card">${mark}<h1>${title}</h1></article>`;
 await writeFile(dir+slug+'.html',html);await page.setContent(html);await page.evaluate(()=>document.fonts.ready);await page.screenshot({path:dir+`png/0${i+1}-${slug}.png`});
}
await browser.close();
await writeFile(dir+'narration.md',cards.map((c,i)=>`### ${i+1}. ${c[1].replaceAll('<br>',' ').replace(/<[^>]+>/g,'')}\n\n${c[3]}\n\nDemo prompt: “${c[2]}”`).join('\n\n'));
