(function () {
  'use strict';

  // Legacy PPT pages contain screenshots and sample numbers. Keep them only
  // in fixture mode; live mode displays authenticated server evidence or an
  // explicit error state.
  var root = document.documentElement;

  function apiBase() {
    var configured = window.CONFIG && window.CONFIG.API_BASE_URL;
    return String(configured || window.location.origin || 'http://localhost:8088').replace(/\/$/, '');
  }

  function getMode() {
    return new URLSearchParams(window.location.search).get('mode') === 'fixture'
      ? 'offline_fixture' : 'live';
  }

  function getPage() {
    return document.body && document.body.getAttribute('data-ppt-page') || 'extension';
  }

  function getToken() {
    var value = window.JWT_TOKEN;
    if (typeof value !== 'string' || !value) return '';
    if (typeof window.isOfflineToken === 'function' && window.isOfflineToken(value)) return '';
    return value;
  }

  function escapeHTML(value) {
    return String(value == null ? '' : value).replace(/[&<>"']/g, function (ch) {
      return ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[ch];
    });
  }

  function ensureStyles() {
    if (document.getElementById('pptRuntimeStyles')) return;
    var style = document.createElement('style');
    style.id = 'pptRuntimeStyles';
    style.textContent = '.ppt-live-empty{margin:20px 0;padding:28px;border:1px solid #d9e5ee;border-radius:16px;background:#fff;color:#183b56;box-shadow:0 8px 28px rgba(24,59,86,.10)}.ppt-live-empty h2{margin:0 0 8px;font-size:20px}.ppt-live-empty p{margin:6px 0;color:#5f7182;line-height:1.6}.ppt-live-empty code{font-family:ui-monospace,monospace;color:#087f8c;white-space:pre-wrap}.ppt-runtime-badge{display:inline-flex;align-items:center;gap:6px;padding:4px 10px;border-radius:999px;background:#e8f5f3;color:#086b75;font-size:12px;font-weight:700}.ppt-runtime-badge.fixture{background:#fff5e5;color:#8a5a13}.ppt-runtime-error{background:#fff1f0;border-color:#f0b9b4;color:#8b2b24}';
    document.head.appendChild(style);
  }

  function installPanel(mode) {
    if (document.getElementById('pptRuntimeStatus')) return document.getElementById('pptRuntimeStatus');
    var panel = document.createElement('aside');
    panel.id = 'pptRuntimeStatus';
    panel.setAttribute('aria-live', 'polite');
    panel.style.cssText = 'position:fixed;right:18px;bottom:18px;z-index:9999;max-width:380px;padding:12px 14px;border:1px solid #d8e5ea;border-radius:12px;background:#f8fbfc;color:#183b56;box-shadow:0 8px 24px rgba(24,59,86,.14);font:13px/1.5 system-ui,sans-serif';
    document.body.appendChild(panel);
    panel.innerHTML = mode === 'offline_fixture'
      ? '<span class="ppt-runtime-badge fixture">offline_fixture</span><br><span>仅展示静态样例；不会联网、调用模型、写入存储或执行删除。</span>'
      : '<span class="ppt-runtime-badge">live</span><br><span>正在等待受保护服务端证据……</span>';
    return panel;
  }

  function replaceLiveContent(page, title, detail, error) {
    var container = document.querySelector('.container') || document.body;
    Array.prototype.forEach.call(container.querySelectorAll('.main-content'), function (node) {
      node.setAttribute('aria-hidden', 'true');
      node.style.display = 'none';
    });
    var existing = document.getElementById('pptLiveEvidence');
    if (existing) existing.remove();
    var card = document.createElement('section');
    card.id = 'pptLiveEvidence';
    card.className = 'ppt-live-empty' + (error ? ' ppt-runtime-error' : '');
    card.innerHTML = '<h2>' + escapeHTML(title) + '</h2><p>' + escapeHTML(detail) + '</p><p><span class="ppt-runtime-badge">live</span> 页面只显示本次受保护请求返回的证据；旧版静态样例已隐藏。</p>';
    container.appendChild(card);
  }

  function unwrap(body) {
    if (body && body.data && typeof body.data === 'object') return body.data;
    return body || {};
  }

  function fetchJSON(path, headers) {
    return fetch(apiBase() + path, { headers: headers }).then(function (response) {
      return response.json().catch(function () { return {}; }).then(function (body) {
        if (!response.ok) {
          var message = body && (body.error || body.message || body.status);
          throw new Error(message || ('HTTP ' + response.status));
        }
        return body;
      });
    });
  }

  function installMultimodalControls(card, headers) {
    var controls = document.createElement('div');
    controls.style.cssText = 'margin-top:18px;padding-top:16px;border-top:1px solid #e4edf2;display:grid;gap:10px';
    controls.innerHTML = '<label for="pptImageUpload"><strong>图片脱敏验证</strong></label><input id="pptImageUpload" type="file" accept="image/png,image/jpeg"><button id="pptImageSubmit" type="button" style="width:max-content;padding:9px 14px;border:0;border-radius:9px;background:#087f8c;color:#fff;cursor:pointer">在内存中处理</button><div id="pptImageResult" role="status" style="color:#5f7182"></div><strong style="margin-top:8px">音频隐私分析</strong><button id="pptAudioSubmit" type="button" style="width:max-content;padding:9px 14px;border:0;border-radius:9px;background:#355c7d;color:#fff;cursor:pointer">生成短测试音频并分析</button><div id="pptAudioResult" role="status" style="color:#5f7182"></div>';
    card.appendChild(controls);
    var input = controls.querySelector('#pptImageUpload');
    var button = controls.querySelector('#pptImageSubmit');
    var result = controls.querySelector('#pptImageResult');
    var audioButton = controls.querySelector('#pptAudioSubmit');
    var audioResult = controls.querySelector('#pptAudioResult');
    button.addEventListener('click', function () {
      if (!input.files || !input.files[0]) {
        result.textContent = '请选择 PNG 或 JPEG 文件。';
        return;
      }
      var form = new FormData();
      form.append('image', input.files[0]);
      button.disabled = true;
      result.textContent = '处理中；原始文件不会上传到持久化存储。';
      fetch(apiBase() + '/api/v1/multimodal/image/deidentify', {
        method: 'POST', headers: headers, body: form
      }).then(function (response) {
        return response.json().catch(function () { return {}; }).then(function (body) {
          if (!response.ok || body.success === false) throw new Error(body.error || body.message || ('HTTP ' + response.status));
          return body;
        });
      }).then(function (body) {
        result.textContent = '已完成内存脱敏 · trace_id: ' + (body.trace_id || '响应未提供') + ' · applied_regions: ' + (body.applied_regions || 0) + ' · persisted: ' + String(body.persisted === true);
        if (body.deidentified_image) {
          var image = document.createElement('img');
          image.alt = '脱敏后的图片副本';
          image.style.cssText = 'display:block;max-width:100%;max-height:280px;border-radius:10px;border:1px solid #d9e5ee;margin-top:8px';
          image.src = body.deidentified_image;
          result.appendChild(image);
        }
      }).catch(function (error) {
        result.textContent = '真实处理失败：' + (error.message || '服务不可用');
      }).finally(function () {
        button.disabled = false;
      });
    });
    audioButton.addEventListener('click', function () {
      var sampleRate = 16000;
      var sampleCount = Math.floor(sampleRate * 0.5);
      var bytes = new Uint8Array(sampleCount * 2);
      var view = new DataView(bytes.buffer);
      for (var i = 0; i < sampleCount; i++) {
        view.setInt16(i * 2, Math.round(Math.sin(i * 2 * Math.PI * 440 / sampleRate) * 12000), true);
      }
      var binary = '';
      for (var j = 0; j < bytes.length; j++) binary += String.fromCharCode(bytes[j]);
      audioButton.disabled = true;
      audioResult.textContent = '处理中；仅提交内存中的 0.5 秒 PCM 摘要。';
      fetch(apiBase() + '/api/v1/multimodal/audio/privacy-analysis', {
        method: 'POST', headers: Object.assign({ 'Content-Type': 'application/json' }, headers),
        body: JSON.stringify({ pcm_base64: btoa(binary), sample_rate: sampleRate, encoding: 'pcm_s16le' })
      }).then(function (response) {
        return response.json().catch(function () { return {}; }).then(function (body) {
          if (!response.ok || body.success === false) throw new Error(body.error || body.message || ('HTTP ' + response.status));
          return body;
        });
      }).then(function (body) {
        audioResult.textContent = '已完成内存分析 · trace_id: ' + (body.trace_id || '响应未提供') + ' · persisted: ' + String(body.persisted === true) + ' · raw_pcm_returned: ' + String(body.raw_pcm_returned === true);
      }).catch(function (error) {
        audioResult.textContent = '真实处理失败：' + (error.message || '服务不可用');
      }).finally(function () {
        audioButton.disabled = false;
      });
    });
  }

  function installLive(page, panel) {
    var bearer = getToken();
    if (!bearer) {
      panel.innerHTML = '<span class="ppt-runtime-badge">live</span><br><strong>需要登录后查看实时证据</strong><br><span>未发现有效 JWT；没有回退到静态成功结果。</span>';
      replaceLiveContent(page, '实时证据不可用', '当前页面需要登录态才能读取受保护 API。请从比赛门户进入，或使用 ?mode=fixture 查看离线样例。', true);
      return;
    }

    var headers = { Authorization: 'Bearer ' + bearer, 'X-Trace-ID': 'ppt-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 8) };
    fetchJSON('/api/v1/lifecycle/status', headers).then(function (body) {
      var data = unwrap(body);
      var status = data.status || {};
      var implementation = status.implementation || data.implementation || 'unknown';
      panel.innerHTML = '<span class="ppt-runtime-badge">live</span><br><strong>服务端证据已返回</strong><br><span>trace_id: <code>' + escapeHTML(body.trace_id || '响应未提供') + '</code></span><br><span>implementation: <code>' + escapeHTML(implementation) + '</code></span>';

      if (page === 'memory_graph') {
        return fetchJSON('/api/v1/evidence/communities', headers).then(function (communityBody) {
          var communityData = unwrap(communityBody);
          var count = Array.isArray(communityData.communities) ? communityData.communities.length : 0;
          replaceLiveContent(page, '记忆子图实时证据', 'owner-scoped 社区数量：' + count + '。页面不展示未授权的节点原文；详细实体与关系应从受保护 evidence API 获取。');
          panel.innerHTML += '<br><span>communities: ' + count + ' · owner scope: authenticated_user</span>';
        });
      }
      if (page === 'lifecycle') {
        var statusJSON = JSON.stringify(status, null, 2);
        replaceLiveContent(page, '冷热分层实时状态', '生命周期接口已返回服务端状态。当前页面仅显示实时 JSON，不把静态图表当作实测指标。\n' + statusJSON);
        return;
      }
      if (page === 'audit') {
        replaceLiveContent(page, '黑匣子实时查询尚未配置', '运行时尚未提供受保护 trace 查询接口，因此不显示旧版审计记录。请从比赛控制台查看本次请求的 trace_id。');
        return;
      }
      if (page === 'multimodal') {
        replaceLiveContent(page, '多模态实时能力待操作', '服务可用，但图像/音频处理必须由用户通过受保护 POST 接口触发。页面不会伪造固定识别率或历史时间戳。');
        installMultimodalControls(document.getElementById('pptLiveEvidence'), headers);
        return;
      }
      replaceLiveContent(page, '实时服务状态', '服务端已响应，但该页面没有可展示的实时证据接口。');
    }).catch(function (error) {
      panel.innerHTML = '<span class="ppt-runtime-badge">live</span><br><strong>无法获取实时证据</strong><br><span>' + escapeHTML(error.message || '服务不可用') + '</span>';
      replaceLiveContent(page, '实时证据不可用', '服务、认证或 Docker 依赖未就绪。页面保留真实错误，不回退到写死的成功数据。', true);
    });
  }

  function install() {
    if (!document.body) return;
    ensureStyles();
    var currentMode = getMode();
    root.setAttribute('data-runtime-mode', currentMode);
    document.body.setAttribute('data-runtime-mode', currentMode);
    var panel = installPanel(currentMode);
    if (currentMode === 'live') installLive(getPage(), panel);
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', install);
  else install();
}());
