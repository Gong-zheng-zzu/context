package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/contextkeeper/service/internal/llm"
)

// selectionUnsetEnv 临时移除指定环境变量，并在测试结束时恢复，保证测试相互隔离。
func selectionUnsetEnv(t *testing.T, keys ...string) {
	t.Helper()

	type savedEnv struct {
		value   string
		present bool
	}
	originals := make(map[string]savedEnv, len(keys))
	for _, key := range keys {
		value, present := os.LookupEnv(key)
		originals[key] = savedEnv{value: value, present: present}
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
	}

	t.Cleanup(func() {
		for _, key := range keys {
			original := originals[key]
			if original.present {
				_ = os.Setenv(key, original.value)
			} else {
				_ = os.Unsetenv(key)
			}
		}
	})
}

// selectionWriteConfig 将内容写入临时配置文件并返回路径。
func selectionWriteConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "llm_config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

// 环境变量已设置时，必须优先使用环境变量，忽略配置文件。
func TestLoadLLMDrivenConfig_EnvOverridesConfigFile(t *testing.T) {
	selectionUnsetEnv(t, "LLM_PROVIDER", "LLM_MODEL")

	path := selectionWriteConfig(t, `
llm:
  default:
    primary_provider: "config_provider"
  providers:
    config_provider:
      model: "config-model"
`)
	t.Setenv(llm.DefaultConfigPathEnv, path)
	t.Setenv("LLM_PROVIDER", "env_provider")
	t.Setenv("LLM_MODEL", "env-model")

	cfg := loadLLMDrivenConfig()
	if cfg.LLM.Provider != "env_provider" || cfg.LLM.Model != "env-model" {
		t.Fatalf("environment variables should win, got %q/%q", cfg.LLM.Provider, cfg.LLM.Model)
	}
}

// 环境变量缺失时，回退读取 config/llm_config.yaml 中的默认 provider/model。
func TestLoadLLMDrivenConfig_FallsBackToConfigFile(t *testing.T) {
	selectionUnsetEnv(t, "LLM_PROVIDER", "LLM_MODEL", llm.DefaultConfigEnvName)

	path := selectionWriteConfig(t, `
llm:
  default:
    primary_provider: "ollama_local"
  providers:
    ollama_local:
      model: "deepseek-coder-v2:16b"
`)
	t.Setenv(llm.DefaultConfigPathEnv, path)

	cfg := loadLLMDrivenConfig()
	if cfg.LLM.Provider != "ollama_local" || cfg.LLM.Model != "deepseek-coder-v2:16b" {
		t.Fatalf("config file fallback failed, got %q/%q", cfg.LLM.Provider, cfg.LLM.Model)
	}
}

// 环境变量与配置文件都取不到时，回退到内置默认值 deepseek/deepseek-chat。
func TestLoadLLMDrivenConfig_FallsBackToBuiltinDefaults(t *testing.T) {
	selectionUnsetEnv(t, "LLM_PROVIDER", "LLM_MODEL", llm.DefaultConfigEnvName)

	path := selectionWriteConfig(t, `
providers:
  ollama_local:
    priority: 1
`)
	t.Setenv(llm.DefaultConfigPathEnv, path)

	cfg := loadLLMDrivenConfig()
	if cfg.LLM.Provider != "deepseek" || cfg.LLM.Model != "deepseek-chat" {
		t.Fatalf("builtin default fallback failed, got %q/%q", cfg.LLM.Provider, cfg.LLM.Model)
	}
}

// 环境变量缺失且配置文件为扁平形态（仓库真实结构）时，应取 environments.<env> 的配置。
func TestLoadLLMDrivenConfig_FallsBackToFlatEnvironments(t *testing.T) {
	selectionUnsetEnv(t, "LLM_PROVIDER", "LLM_MODEL", llm.DefaultConfigEnvName)

	path := selectionWriteConfig(t, `
models:
  local_models:
    - name: "deepseek-coder-v2:16b"
      provider: "ollama_local"
environments:
  production:
    active_providers: ["ollama_local", "deepseek"]
    default_model: "deepseek-coder-v2:16b"
`)
	t.Setenv(llm.DefaultConfigPathEnv, path)

	cfg := loadLLMDrivenConfig()
	if cfg.LLM.Provider != "ollama_local" || cfg.LLM.Model != "deepseek-coder-v2:16b" {
		t.Fatalf("flat environments fallback failed, got %q/%q", cfg.LLM.Provider, cfg.LLM.Model)
	}
}

// LLM_CONFIG_ENV 可切换扁平形态下使用的 environments profile。
func TestLoadLLMDrivenConfig_HonorsConfigEnvProfile(t *testing.T) {
	selectionUnsetEnv(t, "LLM_PROVIDER", "LLM_MODEL", llm.DefaultConfigEnvName)

	path := selectionWriteConfig(t, `
environments:
  development:
    active_providers: ["ollama_local"]
    default_model: "codeqwen:7b"
  production:
    active_providers: ["deepseek"]
    default_model: "deepseek-chat"
`)
	t.Setenv(llm.DefaultConfigPathEnv, path)
	t.Setenv(llm.DefaultConfigEnvName, "development")

	cfg := loadLLMDrivenConfig()
	if cfg.LLM.Provider != "ollama_local" || cfg.LLM.Model != "codeqwen:7b" {
		t.Fatalf("LLM_CONFIG_ENV profile not honored, got %q/%q", cfg.LLM.Provider, cfg.LLM.Model)
	}
}
