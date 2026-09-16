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

  return { normalizeRRFResponse, SOURCE_NAMES: SOURCE_NAMES.slice() };
}));
