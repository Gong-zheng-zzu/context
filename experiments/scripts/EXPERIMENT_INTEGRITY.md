# 实验双哈希与签名元数据

`experiment_integrity.py` 始终输出标准库 SHA-256。若安装了可选依赖 `gmssl`，同时输出真实 SM3；未安装时，SM3 字段为 `unavailable` 并含 `configuration_error`，绝不会用 SHA-256 或其他摘要冒充 SM3。设置 `EVAL_REQUIRE_SM3=true` 会使缺少 `gmssl` 的命令失败。

实验 manifest 保留原有 `sha256` 字段以兼容现有工具，并新增 `hash_algorithms` 与 `dual_digest` 字段。SM3 不可用时，产物不可宣称具备双哈希或国密摘要能力。

可选外部签名器通过 `EVAL_AUDIT_SIGNER_CMD` 显式启用。命令从标准输入读取规范化 JSON 请求，并必须输出 JSON：`signature`、`public_key_id`、`verification_status: "verified"`。未配置时，manifest 写入 `unsigned_not_eligible_for_signed_claim`；签名器失败或验签状态缺失时写入 `signing_failed`。该脚本不会生成或伪造 SM2 密钥、签名或验签结果。
