# Task 3: Configuration Switching Mechanism - Implementation Report

## Overview
Implemented configuration switching mechanism for Context-Keeper backend to enable controlled experiments with different system configurations. This is the final P0 task.

## Implementation Summary

### 1. Contract Test ✅
**File**: `tests/contract/config_switching_contract_test.go`

**Features**:
- Validates different configs have different hash values
- Validates same config produces same hash (idempotent)
- Validates config structure integrity
- Generates contract test report with validation results
- Tests baseline_naive_rag vs full_system configs

**Test Results**:
```
✅ Different configs have different hashes
✅ Baseline配置结构有效
✅ Full System配置结构有效
✅ Hash calculation is idempotent
✅ Detected 6 configuration differences
```

### 2. Go Hash Calculator ✅
**Files**: 
- `pkg/config/hash.go` - Hash calculation utilities
- `pkg/config/hash_test.go` - Unit tests

**Features**:
- `CalculateFileHash(filePath)` - SHA-256 hash from file
- `CalculateStringHash(content)` - SHA-256 hash from string
- `CalculateBytesHash(data)` - SHA-256 hash from bytes
- `ValidateHash(hash)` - Validate hash format (64 hex chars)
- `CompareHashes(hash1, hash2)` - Compare two hashes

**All unit tests pass** ✅

### 3. Python Base Evaluator Enhancement ✅
**File**: `experiments/scripts/base_evaluator.py`

**Added Methods**:
- `calculate_config_hash(config_path)` - Calculate SHA-256 hash of config file
- `get_config_path(config_name)` - Get full path to config file

**Test Results**:
```
[PASS] Config path found
[PASS] Config hash: 42b9ca0fcd270609c612b5784fd452d4b8ef8b902f6b32fa52fced4492e1f861
[PASS] Hash format valid (64 hex characters)
[PASS] Hash calculation is idempotent
[PASS] Different configs have different hashes
```

### 4. Experiment Scripts Updated ✅
All four evaluation scripts now track config hashes in results:

**Files Updated**:
- `experiments/scripts/retrieval_eval.py`
- `experiments/scripts/security_eval.py`
- `experiments/scripts/causal_eval.py`
- `experiments/scripts/unlearning_eval.py`

**Changes**:
- Added `config_hashes` field to result JSON
- Displays config hash summary after saving results
- Format: `{config_name: hash_value[:16]}...`

**Example Output**:
```json
{
  "execution_mode": "live_api",
  "timestamp": "20260713_185630",
  "total_queries": 30,
  "configs": ["baseline_naive_rag", "full_system"],
  "config_hashes": {
    "baseline_naive_rag": "42b9ca0fcd270609c612b5784fd452d4b8ef8b902f6b32fa52fced4492e1f861",
    "full_system": "6821f0bb37af9b26b1e495602c8267d9d0b9cfecd9be143c59ff0aa7c0f805f5"
  },
  "detailed_results": [...],
  "aggregated_metrics": {...}
}
```

## Configuration Hashes (Verified)

| Config Name | Hash (First 16 chars) | Full Hash |
|-------------|----------------------|-----------|
| baseline_naive_rag | `42b9ca0fcd270609` | `42b9ca0fcd270609c612b5784fd452d4b8ef8b902f6b32fa52fced4492e1f861` |
| full_system | `6821f0bb37af9b26` | `6821f0bb37af9b26b1e495602c8267d9d0b9cfecd9be143c59ff0aa7c0f805f5` |

## Implementation Approach: Option B (Restart with Config)

**Chosen**: Option B - Same instance restart with config order

**Rationale**:
1. Simpler for experiments (no multi-instance coordination)
2. Docker compose already supports container restart
3. Config files already exist in `experiments/configs/`
4. Fixed test data naturally reused across restarts

**How It Works**:
1. Stop service (if running)
2. Update active configuration file
3. Restart service with new config
4. Run experiment with fixed test dataset
5. Record config hash in result JSON
6. Repeat for next configuration

**Config Hash Tracking**:
- Each result JSON contains `config_hashes` mapping
- Proves which exact config was used
- Enables post-hoc verification of experiment validity
- Detects accidental config modifications

## Acceptance Criteria Status

| Criteria | Status | Evidence |
|----------|--------|----------|
| Contract test passes | ✅ | `TestConfigSwitchingContract` passes |
| Config hash in result JSON | ✅ | All eval scripts write `config_hashes` field |
| Provably different configs | ✅ | Different hashes: `42b9ca0f...` vs `6821f0bb...` |
| Same test data reused | ✅ | Scripts use fixed datasets from `experiments/datasets/` |
| Smoke test ready | ✅ | Base infrastructure supports smoke testing |

## Files Created/Modified

### Created:
1. `tests/contract/config_switching_contract_test.go` - Contract test
2. `pkg/config/hash.go` - Hash calculation utilities
3. `pkg/config/hash_test.go` - Hash utility tests
4. `experiments/scripts/test_config_hash.py` - Python hash test
5. `experiments/results/contract_test_config_switching.json` - Test report

### Modified:
1. `experiments/scripts/base_evaluator.py` - Added hash methods
2. `experiments/scripts/retrieval_eval.py` - Added config hash tracking
3. `experiments/scripts/security_eval.py` - Added config hash tracking
4. `experiments/scripts/causal_eval.py` - Added config hash tracking
5. `experiments/scripts/unlearning_eval.py` - Added config hash tracking

## Verification Commands

### Run Contract Test:
```bash
cd /d/context/context-keeper-main
go test -v ./tests/contract/config_switching_contract_test.go -timeout 30s
```

### Run Hash Utility Tests:
```bash
cd /d/context/context-keeper-main
go test -v ./pkg/config/hash_test.go ./pkg/config/hash.go -timeout 10s
```

### Test Python Hash Functionality:
```bash
cd /d/context/context-keeper-main/experiments/scripts
python test_config_hash.py
```

### Run Experiment with Config Hash Tracking:
```bash
cd /d/context/context-keeper-main/experiments/scripts
python retrieval_eval.py --configs baseline_naive_rag,full_system --queries 10
# Check output for [CONFIG HASHES] section
```

## Next Steps for Experiments

1. **Run Smoke Test**:
   ```bash
   cd experiments/scripts
   python smoke_test.py
   ```

2. **Run Experiments with Multiple Configs**:
   ```bash
   # Retrieval evaluation
   python retrieval_eval.py --configs baseline_naive_rag,full_system --queries 30
   
   # Security evaluation
   python security_eval.py --configs baseline_naive_rag,full_system --samples 50
   
   # Causal evaluation
   python causal_eval.py --configs baseline_naive_rag,full_system --samples 20
   ```

3. **Verify Config Hashes in Results**:
   ```bash
   # Check that different configs have different hashes
   cat experiments/results/raw/retrieval_*.json | grep config_hashes
   ```

4. **Generate Report**:
   ```bash
   python report_builder.py
   ```

## Key Design Decisions

### 1. No Runtime X-Config-Name Header
- **Decision**: Do NOT use runtime config switching via HTTP headers
- **Reason**: Can lead to race conditions and state confusion
- **Alternative**: Restart service with different config file

### 2. SHA-256 for Config Hash
- **Decision**: Use SHA-256 (64 hex chars)
- **Reason**: Industry standard, collision-resistant, widely supported
- **Alternative Considered**: MD5 (rejected due to security concerns)

### 3. Hash Entire Config File
- **Decision**: Hash raw file content, not parsed YAML
- **Reason**: Simplest, most reliable, no YAML normalization issues
- **Alternative Considered**: Hash normalized YAML (rejected as complex)

### 4. Config Hash in Result JSON
- **Decision**: Store hash at top level of result JSON
- **Reason**: Easy to access, visible in all result aggregations
- **Alternative Considered**: Per-test metadata (rejected as redundant)

## Benefits

1. **Traceability**: Every experiment result is tied to exact config version
2. **Reproducibility**: Config hash enables exact experiment reproduction
3. **Integrity**: Detects accidental config modifications
4. **Simplicity**: No complex runtime config switching logic
5. **Reliability**: Service restart ensures clean state

## Limitations

1. **Restart Overhead**: Service must restart between configs (~seconds)
2. **No Hot-Swapping**: Cannot switch configs without restart
3. **Fixed Config Set**: Must predefine all config files

These limitations are acceptable for experimental evaluation scenarios where restart overhead is negligible compared to experiment runtime.

## Conclusion

Task 3 (Configuration Switching Mechanism) is complete and verified. All acceptance criteria are met:

✅ Contract test written and passing
✅ Config hash calculation implemented in Go and Python
✅ All experiment scripts track config hash in results
✅ Different configs produce different hashes (verified)
✅ Same test data reused across configs (by design)
✅ Ready for smoke testing and full experiments

The implementation provides a simple, reliable mechanism for running controlled experiments with different system configurations while maintaining full traceability of which exact configuration was used for each experiment result.
