#!/usr/bin/env node
/* Render the caregiver page locally at desktop and mobile widths without
 * authentication or API requests. Intended for release-time visual QA. */
'use strict';

const fs = require('fs');
const http = require('http');
const os = require('os');
const path = require('path');
const puppeteer = require('puppeteer');

const webRoot = path.resolve(__dirname, '..', '..', 'web');
const outputDir = fs.mkdtempSync(path.join(os.tmpdir(), 'contextkeeper-caregiver-visual-'));
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
    const relative = pathname === '/' ? 'caregiver_chat.html' : pathname.replace(/^\/+/, '');
    const filePath = path.resolve(webRoot, relative);
    if (!filePath.startsWith(webRoot + path.sep)) {
        response.writeHead(403).end();
        return;
    }
    fs.readFile(filePath, (error, content) => {
        if (error) {
            response.writeHead(404).end();
            return;
        }
        response.writeHead(200, { 'Content-Type': contentType(filePath) });
        response.end(content);
    });
});

async function screenshot(page, viewport, filename) {
    await page.setViewport(viewport);
    await page.goto(`http://127.0.0.1:${server.address().port}/caregiver_chat.html?api=http://127.0.0.1:1`, {
        waitUntil: 'networkidle0'
    });
    await page.evaluate(() => document.getElementById('loginOverlay')?.remove());
    const panel = await page.$('#trustedCareWorkflow');
    if (!panel) throw new Error('trusted caregiver workflow panel is missing');
    const box = await panel.boundingBox();
    if (!box || box.width <= 0 || box.height <= 0) throw new Error('trusted caregiver workflow panel is not visible');
    await page.screenshot({ path: path.join(outputDir, filename), fullPage: true });
}

async function verifyInteractiveBridge(page) {
    const calls = [];
    await page.setRequestInterception(true);
    page.on('request', request => {
        const url = new URL(request.url());
        if (url.host !== '127.0.0.1:1') {
            request.continue();
            return;
        }
        if (request.method() === 'OPTIONS') {
            request.respond({
                status: 204,
                headers: {
                    'Access-Control-Allow-Origin': '*',
                    'Access-Control-Allow-Methods': 'GET, POST, OPTIONS',
                    'Access-Control-Allow-Headers': 'Authorization, Content-Type, X-Trace-ID'
                }
            });
            return;
        }
        calls.push(url.pathname);
        const responses = {
            '/api/v1/causal/extract': {
                relations: [{ mediator: '服用降压药', property: '头晕', result: '跌倒风险', confidence: 0.88 }],
                quality: { tuple_valid: true, review_required: false, confidence_level: 'verified' }
            },
            '/api/mcp/tools/retrieve_context': {
                contexts: [{ content: '近三日血压平稳，夜间起身需协助。' }]
            },
            '/api/v1/agent/execute': {
                success: true,
                data: {
                    answer: '已在当前授权会话内整理护理要点。',
                    write_applied: false,
                    execution_mode: 'read_only_authoritative_tools',
                    session_id: 'session-test',
                    execution: { tool_calls: 1, total_time_ms: 4, steps: [] }
                }
            }
        };
        const body = responses[url.pathname];
        if (!body) {
            request.respond({ status: 404, contentType: 'application/json', headers: { 'Access-Control-Allow-Origin': '*' }, body: JSON.stringify({ error: 'unexpected endpoint' }) });
            return;
        }
        request.respond({ status: 200, contentType: 'application/json', headers: { 'Access-Control-Allow-Origin': '*' }, body: JSON.stringify(body) });
    });

    await page.evaluate(() => {
        JWT_TOKEN = 'browser-test-token';
        currentSessionID = 'session-test';
        window.dispatchEvent(new CustomEvent('caregiver:record-saved', {
            detail: { content: '服用降压药后头晕，起身时不稳。', sessionID: 'session-test' }
        }));
    });
    await page.waitForFunction(() => document.getElementById('trustedCareResult').textContent.includes('跌倒风险'), { timeout: 5000 });
    await page.click('#trustedCareRetrieve');
    await page.waitForFunction(() => document.getElementById('trustedCareResult').textContent.includes('近三日血压平稳'), { timeout: 5000 });
    const agent = await page.evaluate(() => window.runCaregiverTrustedAgent({
        message: '整理当前护理要点', sessionID: 'session-test'
    }));
    if (!agent || agent.answer !== '已在当前授权会话内整理护理要点。') {
        throw new Error('controlled caregiver Agent did not return the expected read-only response');
    }
    for (const path of ['/api/v1/causal/extract', '/api/mcp/tools/retrieve_context', '/api/v1/agent/execute']) {
        if (!calls.includes(path)) throw new Error(`caregiver bridge did not call ${path}`);
    }
}

async function main() {
    await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
    if (!browserPath) throw new Error('No local Chrome/Edge executable is available for visual verification');
    const browser = await puppeteer.launch({ headless: true, executablePath: browserPath, args: ['--no-sandbox'] });
    try {
        const page = await browser.newPage();
        await screenshot(page, { width: 1440, height: 1000, deviceScaleFactor: 1 }, 'caregiver-desktop.png');
        await screenshot(page, { width: 390, height: 844, deviceScaleFactor: 1 }, 'caregiver-mobile.png');
        await verifyInteractiveBridge(page);
        console.log(JSON.stringify({ status: 'passed', output_dir: outputDir }));
    } finally {
        await browser.close();
        await new Promise(resolve => server.close(resolve));
    }
}

main().catch(error => {
    console.error(error.stack || String(error));
    server.close(() => process.exit(1));
});
