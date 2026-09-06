# Causal Canonical Evaluation Report

## Run

- Timestamp: `20260717_111716`
- Evaluator: `experiments/scripts/causal_eval.py`
- Run label: `causal_extract_canonical`
- Fixture: `testdata/causal_test_cases.json`
- Requested samples: `20`
- Requested minimum confidence: `0.01`
- API base URL: `http://localhost:8088`
- Health check: `200 {"status":"healthy"}`
- Extraction endpoint: `POST /api/v1/causal/extract`
- Raw result: `experiments/results/raw/causal_20260717_111716.json`

## Metrics

| Metric | Value |
| --- | ---: |
| Total annotated cases | 20 |
| API/contract-valid responses | 0 |
| API/contract errors | 20 |
| Relation found count | 0 |
| Object match count | 0 |
| Mediator match count | 0 |
| Property match count | 0 |
| Result match count | 0 |
| Strict O-M-P-R tuple matches | 0 |
| Strict tuple plus confidence-threshold passes | 0 |
| Relation found rate | n/a |
| Object accuracy | n/a |
| Mediator accuracy | n/a |
| Property accuracy | n/a |
| Result accuracy | n/a |
| Strict O-M-P-R tuple accuracy | n/a |
| Strict tuple plus confidence-threshold accuracy | n/a |
| Average best-relation confidence | n/a |
| Average latency for valid responses | n/a |

## API Failures

All 20 extraction requests received `HTTP 404: 404 page not found`. These are API failures and are excluded from the business-metric denominator. This run does not establish extraction-quality performance.
