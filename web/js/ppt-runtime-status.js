(function () {
  'use strict';

  // Shared status bridge for the legacy PPT pages. It never invents metrics and
  // never stores credentials. A live status is shown only after a real,
  // authenticated response; otherwise the page remains explicitly labelled.
  function apiBase() {
    return window.CONFIG && window.CONFIG.API_BASE_URL
      ? window.CONFIG.API_BASE_URL.replace(/\/$/, '')
      : 'http://localhost:8088';
  }

  function token() {
    return typeof window.JWT_TOKEN === 'string' && window.JWT_TOKEN &&
      !(typeof window.isOfflineToken === 'function' && window.isOfflineToken(window.JWT_TOKEN))
      ? window.JWT_TOKEN : '';
  }

  function mode() {
    return new URLSearchParams(window.location.search).get('mode') === 'fixture'
      ? 'offline_fixture' : 'live';
  }

  function pageKind() {
    const path = window.location.pathname;
    if (path.includes('黑匣子')) return 'audit';
    if (path.includes('数据调度')) return 'lifecycle';
    if (path.includes('隐私保护')) return 'multimodal';
    if (path.includes('三维协同记忆图')) return 'memory_graph';
    return 'extension';
  }

  function install() {
    if (!document.body || document.getElementById('pptRuntimeStatus')) return;
    const panel = document.createElement('aside');
    panel.id = 'pptRuntimeStatus';
    panel.setAttribute('aria-live', 'polite');
    panel.style.cssText = 'position:fixed;right:18px;bottom:18px;z-index:9999;max-width:360px;padding:12px 14px;border:1px solid #d8e5ea;border-radius:12px;background:#f8fbfc;color:#183b56;box-shadow:0 8px 24px rgba(24,59,86,.14);font:13px/1.5 system-ui,sans-serif';
    document.body.appendChild(panel);
    const currentMode = mode();
    if (currentMode === 'offline_fixture') {
      panel.innerHTML = '<strong>offline_fixture · 受控演示</strong><br><span>不调用服务、模型、数据库、联网 Agent、写入或删除接口。</span>';
      return;
    }
    panel.innerHTML = '<strong>live · 等待受保护响应</strong><br><span>登录后将显示服务端真实能力状态。</span>';
    const bearer = token();
    if (!bearer) return;
    const headers = { Authorization: `Bearer ${bearer}`, 'X-Trace-ID': `ppt-${Date.now()}-${Math.random().toString(36).slice(2, 8)}` };
    const lifecycle = fetch(`${apiBase()}/api/v1/lifecycle/status`, { headers }).then(r => r.json().then(body => ({ ok: r.ok, body })));
    lifecycle.then(({ ok, body }) => {
      const payload = body && body.data && typeof body.data === 'object' ? body.data : body;
      if (!ok || !payload || payload.success === false) {
        panel.innerHTML = '<strong>live · 服务状态不可用</strong><br><span>页面保留真实错误状态，未回退到写死结果。</span>';
        return;
      }
      const implementation = payload.implementation || 'unknown';
      const prototype = implementation === 'in_process_evidence_lifecycle';
      const extra = pageKind() === 'multimodal'
        ? '图像/音频接口为内存受控处理，需在比赛控制台执行具体验证。'
        : pageKind() === 'audit'
          ? '审计页面不自行生成 Trace 记录，实时证据以受保护接口响应为准。'
          : pageKind() === 'memory_graph'
            ? '记忆子图当前按 owner 隔离，进程内实现不等同于持久化图谱。'
            : '生命周期状态来自服务端，未返回的指标不会在页面显示。';
      if (pageKind() === 'memory_graph') {
        fetch(`${apiBase()}/api/v1/evidence/communities`, { headers }).then(r => r.json().then(communityBody => ({ ok: r.ok, communityBody }))).then(({ ok: communityBody }) => {
          const data = communityBody && communityBody.data && typeof communityBody.data === 'object' ? communityBody.data : communityBody;
          const count = data && Array.isArray(data.communities) ? data.communities.length : null;
          const communityText = ok && count !== null ? `服务端 owner-scoped 社区数：${count}（不展示未授权原文）。` : '无法获取 owner-scoped 社区证据，页面保留静态说明。';
          panel.innerHTML = `<strong>live · ${prototype ? '受控原型' : '服务端状态'}</strong><br><span>implementation: ${String(implementation).replace(/[<>]/g, '')}</span><br><span>${communityText}</span><br><span>${extra}</span>`;
        }).catch(() => {
          panel.innerHTML = `<strong>live · ${prototype ? '受控原型' : '服务端状态'}</strong><br><span>implementation: ${String(implementation).replace(/[<>]/g, '')}</span><br><span>无法获取 owner-scoped 社区证据，页面保留静态说明。</span>`;
        });
        return;
      }
      panel.innerHTML = `<strong>live · ${prototype ? '受控原型' : '服务端状态'}</strong><br><span>implementation: ${String(implementation).replace(/[<>]/g, '')}</span><br><span>${extra}</span>`;
    }).catch(() => {
      panel.innerHTML = '<strong>live · 服务不可用</strong><br><span>无法获取实时证据，请检查服务、认证和 Docker 依赖。</span>';
    });
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', install);
  else install();
}());
