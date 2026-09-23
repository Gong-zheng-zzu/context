#!/usr/bin/env node
/* Render all public UI entries at desktop and mobile widths. The test verifies
 * that the shared local theme loaded and that the viewport is not overflowed. */
'use strict';

const fs = require('fs');
const http = require('http');
const os = require('os');
const path = require('path');
const puppeteer = require('puppeteer');

const webRoot = path.resolve(__dirname, '..', '..', 'web');
const outputDir = fs.mkdtempSync(path.join(os.tmpdir(), 'contextkeeper-role-theme-'));
const pages = ['caregiver_chat.html', 'doctor_chat.html', 'family_chat.html', 'elder_voice_chat.html', 'role_selection.html', 'competition_portal.html'];
const browserPath = process.env.PUPPETEER_EXECUTABLE_PATH || [
    'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
    'C:\\Program Files (x86)\\Google\\Chrome\\Application\\chrome.exe',
    'C:\\Program Files\\Microsoft\\Edge\\Application\\msedge.exe',
    'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'
].find(fs.existsSync);

function contentType(filePath) {
    if (filePath.endsWith('.html')) return 'text/html; charset=utf-8';
    if (filePath.endsWith('.js')) return 'application/javascript; charset=utf-8';
    if (filePath.endsWith('.css')) return 'text/css; charset=utf-8';
    return 'application/octet-stream';
}

const server = http.createServer((request, response) => {
    const pathname = new URL(request.url, 'http://127.0.0.1').pathname;
    const relative = pathname === '/' ? 'role_selection.html' : pathname.replace(/^\/+/, '');
    const filePath = path.resolve(webRoot, relative);
    if (!filePath.startsWith(webRoot + path.sep)) return response.writeHead(403).end();
    fs.readFile(filePath, (error, content) => {
        if (error) return response.writeHead(404).end();
        response.writeHead(200, { 'Content-Type': contentType(filePath) });
        response.end(content);
    });
});

async function assertPage(page, name, viewport) {
    await page.setViewport(viewport);
    await page.goto(`http://127.0.0.1:${server.address().port}/${name}?api=http://127.0.0.1:1&mode=fixture`, { waitUntil: 'networkidle0' });
    await page.evaluate(() => document.getElementById('loginOverlay')?.remove());
    const state = await page.evaluate(() => ({
        themeLoaded: Array.from(document.styleSheets).some(sheet => sheet.href && sheet.href.endsWith('/css/trusted-care-theme.css')),
        viewportWidth: document.documentElement.clientWidth,
        scrollWidth: document.documentElement.scrollWidth,
        visible: document.body.getBoundingClientRect().height > 0
    }));
    if (!state.themeLoaded) throw new Error(`${name}: shared theme did not load`);
    if (!state.visible) throw new Error(`${name}: page body is not visible`);
    if (state.scrollWidth > state.viewportWidth + 2) throw new Error(`${name}: horizontal overflow ${state.scrollWidth}/${state.viewportWidth}`);
    const suffix = viewport.width < 700 ? 'mobile' : 'desktop';
    await page.screenshot({ path: path.join(outputDir, `${name.replace('.html', '')}-${suffix}.png`), fullPage: true });
}

async function main() {
    if (!browserPath) throw new Error('No local Chrome or Edge executable is available');
    await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
    const browser = await puppeteer.launch({ headless: true, executablePath: browserPath, args: ['--no-sandbox'] });
    try {
        const page = await browser.newPage();
        for (const name of pages) {
            await assertPage(page, name, { width: 1440, height: 960, deviceScaleFactor: 1 });
            await assertPage(page, name, { width: 390, height: 844, deviceScaleFactor: 1 });
        }
        console.log(JSON.stringify({ status: 'passed', pages: pages.length, output_dir: outputDir }));
    } finally {
        await browser.close();
        await new Promise(resolve => server.close(resolve));
    }
}

main().catch(error => { console.error(error.stack || String(error)); server.close(() => process.exit(1)); });
