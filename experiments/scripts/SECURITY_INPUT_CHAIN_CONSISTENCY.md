# Security Input Chain Consistency

`security_input_chain_consistency.py` compares only the pre-generation input
security decision made by the lightweight evaluator and the controlled
`evaluationInputOnly` mode of the chat route. It is not a full-chat-text
consistency test.
It uses 12 fixed canonical samples: `_001` and `_002` from each of the six
attack categories. Each pair of requests uses one new isolated session ID.

The JSON evidence stores sample and session SHA-256 values plus decision
metadata. It does not store the attack input, JWT, chat response, user message,
or sensitive-info values. Authentication, health, transport, HTTP, and protocol
errors are recorded separately and produce no consistency conclusion.

```powershell
$env:EVAL_USER_ID = 'eval_user_001'
$env:EVAL_PASSWORD = 'demo123'
python experiments/scripts/security_input_chain_consistency.py --base-url http://127.0.0.1:8088
```

Run the no-network classifier checks:

```powershell
python experiments/scripts/security_input_chain_consistency.py --self-test
```

Evidence is written to `experiments/results/raw/security_input_chain_<UTC>.json`
unless `--output` is provided. A `mismatch` is an observed decision difference.
The evidence records `comparison_mode=chat_pre_generation_input_chain`. An
authentication or service error is `inconclusive_due_to_auth_or_service_error`,
not a match or mismatch conclusion.
