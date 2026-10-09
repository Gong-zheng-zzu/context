(function (root, factory) {
  const api = factory();
  if (typeof module === 'object' && module.exports) module.exports = api;
  if (root) root.CompetitionAudit = api;
}(typeof globalThis !== 'undefined' ? globalThis : this, function () {
  'use strict';

  const SOURCE_NAMES = ['vector', 'knowledge', 'timeline'];

  function object(value) {
    return value && typeof value === 'object' && !Array.isArray(value) ? value : {};
  }

  function first(objectValue, keys, fallback) {
    for (const key of keys) {
      if (objectValue[key] !== undefined && objectValue[key] !== null) return objectValue[key];
    }
    return fallback;
  }

  function numberOrNull(value) {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : null;
  }

  function stringArray(value) {
    return Array.isArray(value) ? value.filter(item => typeof item === 'string' && item.trim()).map(item => item.trim()) : [];
  }

  function normalizeRRFResponse(response) {
    const envelope = object(response);
    const payload = object(envelope.data && typeof envelope.data === 'object' ? envelope.data : envelope);
    const metadata = object(first(payload, ['retrieval_metadata', 'retrievalMetadata'], {}));
    const statuses = object(first(metadata, ['source_statuses', 'retrieval_source_statuses'], {}));
    const latencies = object(first(metadata, ['source_latency_ms', 'retrieval_source_latency_ms'], {}));
    const counts = object(first(metadata, ['source_candidate_counts', 'retrieval_source_candidate_counts'], {}));
    const activeSources = stringArray(first(metadata, ['retrieval_active_sources', 'active_sources'], []));
    const emptySources = stringArray(first(metadata, ['retrieval_empty_sources', 'empty_sources'], []));
    const fusionMode = String(first(metadata, ['retrieval_fusion_mode', 'fusion_mode'], 'unknown'));
    const wallClockLatencyMs = numberOrNull(first(metadata, ['wall_clock_latency_ms', 'retrieval_wall_clock_latency_ms'], null));
    const rawContexts = first(payload, ['contexts', 'retrieved_contexts', 'retrievedContexts'], []);
    const contexts = Array.isArray(rawContexts) ? rawContexts : [];

    const sources = SOURCE_NAMES.map(name => ({
      name,
      status: typeof statuses[name] === 'string' && statuses[name].trim() ? statuses[name].trim() : 'unknown',
      latencyMs: numberOrNull(latencies[name]),
      candidateCount: numberOrNull(counts[name]),
      active: activeSources.includes(name),
      empty: emptySources.includes(name)
    }));

    const results = contexts.map((context, index) => {
      const item = object(context);
      const itemMetadata = object(item.metadata);
      return {
        docId: String(first(itemMetadata, ['doc_id'], first(item, ['doc_id', 'docId', 'id'], `untraceable-${index + 1}`))),
        content: String(first(item, ['content', 'text'], '')),
        score: numberOrNull(first(itemMetadata, ['rrf_score'], first(item, ['score'], null))),
        sources: stringArray(first(itemMetadata, ['rrf_sources'], [])),
        ranks: object(first(itemMetadata, ['rrf_ranks'], {}))
      };
    });

    const hasSourceAudit = SOURCE_NAMES.every(name => Object.prototype.hasOwnProperty.call(statuses, name));
    const hasFusionAudit = fusionMode !== 'unknown';
    return {
      fusionMode,
      wallClockLatencyMs,
      activeSources,
      emptySources,
      sources,
      results,
      auditComplete: hasSourceAudit && hasFusionAudit,
      evidencePresent: results.length > 0
    };
  }

  // The console loads this helper before its inline controller.  The card is
  // deliberately mounted here so the page can expose the PPT capability
  // boundary without duplicating authentication or inventing benchmark data.
  function installEvidenceLevelCard() {
    if (typeof document === 'undefined' || document.getElementById('evidenceLevelCard')) return;
    const badge = document.getElementById('modeBadge');
    const banner = document.querySelector('.banner');
    if (!badge || !banner) return;
    const card = document.createElement('section');
    card.id = 'evidenceLevelCard';
    card.className = 'card evidence-level-card';
    card.setAttribute('aria-label', 'PPT 能力状态与证据等级');
    card.innerHTML = '<h2>能力状态与证据等级</h2>' +
      '<p class="hint">比赛主链以受保护 API 的实际响应为准。此卡片不展示未经评测的准确率，也不把受控原型当作正式能力。</p>' +
      '<div class="evidence-level-grid">' +
      '<div><strong class="evidence-level-main">主链</strong><span>ASDF/CASIA/PCCM-S、安全审计、O-M-P-R、RRF 与删除验证</span></div>' +
      '<div><strong class="evidence-level-prototype">受控原型</strong><span>证据社区、风险排序、多模态最小化处理</span></div>' +
      '<div><strong class="evidence-level-unavailable">未接入</strong><span>未通过冻结评测的训练模型与参数级遗忘</span></div>' +
      '</div><div id="evidenceLifecycleStatus" class="audit-status review">服务端生命周期状态：尚未读取；登录后点击“生命周期状态”获取真实证据。</div>';
    banner.after(card);

    const style = document.createElement('style');
    style.textContent = '.evidence-level-card{border-top-color:#78a98f}.evidence-level-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:10px}.evidence-level-grid>div{border:1px solid var(--line);padding:10px;background:#f8fbfc}.evidence-level-grid strong,.evidence-level-grid span{display:block}.evidence-level-grid span{color:var(--muted);font-size:12px;margin-top:4px}.evidence-level-main{color:var(--green)}.evidence-level-prototype{color:var(--amber)}.evidence-level-unavailable{color:var(--red)}@media(max-width:760px){.evidence-level-grid{grid-template-columns:1fr}}';
    document.head.appendChild(style);

    const status = document.getElementById('evidenceLifecycleStatus');
    const setFixtureState = () => {
      const fixture = document.getElementById('mode')?.value === 'fixture';
      if (fixture) {
        status.textContent = '服务端生命周期状态：离线夹具；仅展示固定样例，不执行联网、写入或检索。';
        status.className = 'audit-status review';
      } else if (!status.dataset.serverState) {
        status.textContent = '服务端生命周期状态：尚未读取；登录后点击“生命周期状态”获取真实证据。';
        status.className = 'audit-status review';
      }
    };
    document.getElementById('mode')?.addEventListener('change', () => { delete status.dataset.serverState; setFixtureState(); });
    setFixtureState();

    const originalFetch = window.fetch.bind(window);
    window.fetch = function evidenceAuditFetch(input, init) {
      return originalFetch(input, init).then(response => {
        const url = typeof input === 'string' ? input : input && input.url || '';
        if (!url.includes('/api/v1/lifecycle/status')) return response;
        response.clone().json().then(body => {
          const payload = body && body.data && typeof body.data === 'object' ? body.data : body;
          const implementation = payload && payload.implementation;
          status.dataset.serverState = implementation || 'unknown';
          if (response.ok && implementation === 'in_process_evidence_lifecycle') {
            status.textContent = '服务端生命周期状态：受控证据生命周期已确认（进程内）；不代表持久化图谱或模型遗忘。';
            status.className = 'audit-status verified';
          } else if (response.ok) {
            status.textContent = `服务端生命周期状态：已返回，但实现标识为 ${implementation || '未声明'}；不能提升证据等级。`;
            status.className = 'audit-status review';
          } else {
            status.textContent = '服务端生命周期状态：请求失败；无法确认实现状态。';
            status.className = 'audit-status failed';
          }
        }).catch(() => {
          status.textContent = '服务端生命周期状态：响应无法解析；不能确认实现状态。';
          status.className = 'audit-status failed';
        });
        return response;
      });
    };
  }

  if (typeof document !== 'undefined') {
    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', installEvidenceLevelCard);
    else installEvidenceLevelCard();
  }

  return { normalizeRRFResponse, SOURCE_NAMES: SOURCE_NAMES.slice(), installEvidenceLevelCard };
}));
