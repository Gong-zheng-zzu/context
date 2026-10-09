/*
 * Caregiver-facing bridge for the audited HTTP capabilities. It intentionally
 * presents concise care-support outcomes rather than raw RRF, graph, or model
 * internals, and it never stores tokens or nursing text in browser storage.
 */
(function () {
    'use strict';

    let lastRecord = null;

    function byID(id) {
        return document.getElementById(id);
    }

    function traceID() {
        if (globalThis.crypto && typeof globalThis.crypto.randomUUID === 'function') {
            return globalThis.crypto.randomUUID();
        }
        return `caregiver_${Date.now()}_${Math.random().toString(36).slice(2, 10)}`;
    }

    function apiBase() {
        return window.CONFIG && window.CONFIG.API_BASE_URL
            ? window.CONFIG.API_BASE_URL.replace(/\/$/, '')
            : 'http://localhost:8088';
    }

    function authToken() {
        return typeof JWT_TOKEN === 'string' ? JWT_TOKEN : '';
    }

    function showResult(message, state) {
        const result = byID('trustedCareResult');
        if (!result) return;
        result.textContent = message;
        result.className = `trusted-workflow-result ${state || ''}`;
        result.style.display = 'block';
    }

    function setStatus(message) {
        const status = byID('trustedCareStatus');
        if (status) status.textContent = message;
    }

    async function request(path, options) {
        const token = authToken();
        if (!token || (typeof isOfflineToken === 'function' && isOfflineToken(token))) {
            throw new Error('请在实时登录状态下使用可信护理辅助。');
        }
        const response = await fetch(`${apiBase()}${path}`, {
            ...options,
            headers: {
                Authorization: `Bearer ${token}`,
                'X-Trace-ID': traceID(),
                ...(options && options.headers ? options.headers : {})
            }
        });
        const raw = await response.text();
        let body = {};
        try { body = raw ? JSON.parse(raw) : {}; } catch (_) { body = { error: raw }; }
        if (!response.ok || body.success === false) {
            throw new Error(body.error || body.message || `请求失败 (${response.status})`);
        }
        return body.data && typeof body.data === 'object' ? body.data : body;
    }

    function firstRelation(payload) {
        return payload && Array.isArray(payload.relations) ? payload.relations[0] : null;
    }

    function decisionLabel(quality) {
        if (!quality) return '未返回质量审计';
        if (quality.confidence_level === 'verified' || quality.review_required === false && quality.tuple_valid) return '证据校验通过';
        if (quality.confidence_level === 'insufficient_evidence') return '证据不足';
        return '需要人工复核';
    }

    async function analyseRecord(record) {
        setStatus('正在生成风险提示');
        try {
            const payload = await request('/api/v1/causal/extract', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    text: record.content,
                    persist: false,
                    use_rules: true,
                    use_pmi: false,
                    use_llm: true,
                    min_confidence: 0.5
                })
            });
            const relation = firstRelation(payload);
            const quality = payload.quality || (relation && relation.quality);
            const decision = decisionLabel(quality);
            if (!relation) {
                showResult(`风险提示：${decision}。本次记录未形成可验证的因果关系，请按护理流程人工确认。`, 'review');
                setStatus('风险提示需要人工确认');
                return;
            }
            const confidence = Number(relation.confidence || 0).toFixed(2);
            const state = quality && (quality.review_required || quality.confidence_level === 'insufficient_evidence') ? 'review' : 'verified';
            showResult(`风险提示：${relation.result || '未返回结果'}\n依据：${relation.mediator || '未返回中介'} -> ${relation.property || '未返回属性'}\n状态：${decision} · 置信度 ${confidence}\n仅供护理辅助与复核，不提供诊断或治疗建议。`, state);
            setStatus(state === 'verified' ? '风险提示已生成' : '风险提示待人工复核');
        } catch (error) {
            showResult(`风险提示未生成：${error.message}`, 'failed');
            setStatus('风险提示请求失败，原记录已安全保存');
        }
    }

    async function retrieveRelatedRecords() {
        if (!lastRecord) {
            showResult('请先成功保存一条护理记录，再查询关联记录。', 'review');
            return;
        }
        setStatus('正在查询已授权关联记录');
        try {
            const payload = await request('/api/mcp/tools/retrieve_context', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    sessionId: lastRecord.sessionID,
                    query: lastRecord.content,
                    maxResults: 3,
                    contextsOnly: true,
                    evaluationRetrievalOnly: true
                })
            });
            const contexts = Array.isArray(payload.contexts) ? payload.contexts : [];
            if (!contexts.length) {
                showResult('未找到已授权的关联记录。该结果不代表不存在其他住民或其他会话的数据。', 'review');
                setStatus('未检索到关联记录');
                return;
            }
            const summaries = contexts.slice(0, 3).map((item, index) => {
                const content = String(item.content || item.summary || '后端未返回记录摘要').replace(/\s+/g, ' ').slice(0, 100);
                return `${index + 1}. ${content}${content.length >= 100 ? '...' : ''}`;
            });
            showResult(`已找到 ${contexts.length} 条当前授权范围内的关联记录：\n${summaries.join('\n')}`, 'verified');
            setStatus('关联记录已返回');
        } catch (error) {
            showResult(`关联记录查询失败：${error.message}`, 'failed');
            setStatus('关联记录查询失败');
        }
    }

    async function deidentifyImage(file) {
        if (!file) return;
        if (!['image/jpeg', 'image/png'].includes(file.type) || file.size > 5 * 1024 * 1024) {
            showResult('仅支持不超过 5 MiB 的 JPEG 或 PNG 护理影像。', 'review');
            return;
        }
        setStatus('正在进行内存脱敏处理');
        try {
            const form = new FormData();
            form.append('image', file);
            const payload = await request('/api/v1/multimodal/image/deidentify', { method: 'POST', body: form });
            if (payload.original_returned !== false || payload.persisted !== false || payload.execution_mode !== 'memory_only') {
                throw new Error('服务端未满足影像最小化处理契约');
            }
            showResult(`影像脱敏完成：已应用 ${Number(payload.applied_regions || 0)} 个处理区域。原始影像未返回、未持久化；请在保存前确认脱敏副本。`, 'verified');
            setStatus('影像脱敏完成，未自动保存');
        } catch (error) {
            showResult(`影像脱敏失败：${error.message}`, 'failed');
            setStatus('影像脱敏失败，原始文件未自动保存');
        }
    }

    function syntheticPCMBase64(sampleRate, seconds) {
        const count = Math.floor(sampleRate * seconds);
        const bytes = new Uint8Array(count * 2);
        for (let index = 0; index < count; index += 1) {
            const sample = Math.round(Math.sin(2 * Math.PI * 220 * index / sampleRate) * 0.18 * 32767);
            const offset = index * 2;
            bytes[offset] = sample & 0xff;
            bytes[offset + 1] = (sample >> 8) & 0xff;
        }
        let binary = '';
        for (let index = 0; index < bytes.length; index += 1) binary += String.fromCharCode(bytes[index]);
        return btoa(binary);
    }

    async function analyseSyntheticAudio() {
        setStatus('正在执行合成音频内存隐私处理');
        try {
            const sampleRate = 16000;
            const payload = await request('/api/v1/multimodal/audio/privacy-analysis', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ encoding: 'pcm_s16le', sample_rate: sampleRate, pcm_base64: syntheticPCMBase64(sampleRate, 1) })
            });
            if (payload.persisted !== false || payload.raw_pcm_returned !== false || payload.raw_pcm_persisted !== false || !payload.transform) {
                throw new Error('服务端未满足音频最小化处理契约');
            }
            showResult(`合成音频隐私处理完成：${payload.transform.algorithm || '已执行'}。原始 PCM 未返回、未持久化。`, 'verified');
            setStatus('音频隐私处理完成，未保存原始音频');
        } catch (error) {
            showResult(`音频隐私处理失败：${error.message}`, 'failed');
            setStatus('音频隐私处理失败，未伪造成功结果');
        }
    }

    window.runCaregiverTrustedAgent = async function runCaregiverTrustedAgent(requestInput) {
        const sessionID = String(requestInput && requestInput.sessionID || '').trim();
        const message = String(requestInput && requestInput.message || '').trim();
        if (!sessionID || !message) throw new Error('受控 Agent 需要有效的当前会话和问题。');
        setStatus('受控 Agent 正在执行只读工具');
        const payload = await request('/api/v1/agent/execute', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ session_id: sessionID, query: message })
        });
        if (payload.write_applied !== false || payload.execution_mode !== 'read_only_authoritative_tools') {
            throw new Error('服务端未确认只读受控 Agent 契约。');
        }
        setStatus('受控 Agent 已完成只读查询');
        return {
            answer: payload.answer || '受控 Agent 未返回答复。',
            execution: payload.execution || null,
            sessionID: payload.session_id || sessionID
        };
    };

    function initialize() {
        const retrieveButton = byID('trustedCareRetrieve');
        const imageButton = byID('trustedCareImage');
        const imageInput = byID('trustedCareImageInput');
        const audioButton = byID('trustedCareAudio');
        if (retrieveButton) retrieveButton.addEventListener('click', retrieveRelatedRecords);
        if (imageButton && imageInput) imageButton.addEventListener('click', () => imageInput.click());
        if (audioButton) audioButton.addEventListener('click', analyseSyntheticAudio);
        if (imageInput) imageInput.addEventListener('change', async () => {
            const file = imageInput.files && imageInput.files[0];
            imageInput.value = '';
            await deidentifyImage(file);
        });
        window.addEventListener('caregiver:record-saved', event => {
            const detail = event && event.detail;
            if (!detail || !detail.content || !detail.sessionID) return;
            lastRecord = { content: String(detail.content), sessionID: String(detail.sessionID) };
            if (retrieveButton) retrieveButton.disabled = false;
            const auto = byID('trustedCareAutoCausal');
            if (auto && auto.checked) analyseRecord(lastRecord);
            else setStatus('护理记录已保存；可查询关联记录');
        });
    }

    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', initialize);
    else initialize();
}());
