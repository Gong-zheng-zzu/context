package llm

import (
	"os"
	"path/filepath"
	"testing"
)

// selectionClearEnv 临时移除指定环境变量，并在测试结束时恢复，保证测试相互隔离。
func selectionClearEnv(t *testing.T, keys ...string) {
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

func TestLoadDefaultLLMSelectionFromFile_NestedForm(t *testing.T) {
	selectionClearEnv(t, DefaultConfigEnvName, DefaultConfigPathEnv)

	path := selectionWriteConfig(t, `
llm:
  default:
    primary_provider: "ollama_local"
  providers:
    ollama_local:
      model: "deepseek-coder-v2:16b"
`)

	selection, err := LoadDefaultLLMSelectionFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selection.Provider != "ollama_local" || selection.Model != "deepseek-coder-v2:16b" {
		t.Fatalf("nested llm section not parsed, got %q/%q", selection.Provider, selection.Model)
	}
	if selection.Source != "llm.default" {
		t.Fatalf("unexpected source, got %q", selection.Source)
	}
}

// 同时存在嵌套 llm: 段与扁平 environments 段时，嵌套 llm: 段必须优先。
func TestLoadDefaultLLMSelectionFromFile_NestedWinsOverFlat(t *testing.T) {
	selectionClearEnv(t, DefaultConfigEnvName, DefaultConfigPathEnv)

	path := selectionWriteConfig(t, `
llm:
  default:
    primary_provider: "nested_provider"
  providers:
    nested_provider:
      model: "nested-model"
environments:
  production:
    active_providers: ["flat_provider"]
    default_model: "flat-model"
`)

	selection, err := LoadDefaultLLMSelectionFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selection.Provider != "nested_provider" || selection.Model != "nested-model" {
		t.Fatalf("nested llm section should win over flat, got %q/%q", selection.Provider, selection.Model)
	}
	if selection.Source != "llm.default" {
		t.Fatalf("unexpected source, got %q", selection.Source)
	}
}

func TestLoadDefaultLLMSelectionFromFile_FlatForm(t *testing.T) {
	selectionClearEnv(t, DefaultConfigEnvName, DefaultConfigPathEnv)

	path := selectionWriteConfig(t, `
providers:
  ollama_local:
    priority: 1
models:
  local_models:
    - name: "deepseek-coder-v2:16b"
      provider: "ollama_local"
  cloud_models:
    - name: "deepseek-chat"
      provider: "deepseek"
environments:
  production:
    active_providers: ["ollama_local", "deepseek"]
    default_model: "deepseek-coder-v2:16b"
`)

	selection, err := LoadDefaultLLMSelectionFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selection.Provider != "ollama_local" || selection.Model != "deepseek-coder-v2:16b" {
		t.Fatalf("flat environments section not resolved, got %q/%q", selection.Provider, selection.Model)
	}
	if selection.Source != "environments.production" {
		t.Fatalf("unexpected source, got %q", selection.Source)
	}
}

// 无 llm: 段、default_model 又不在 models 列表中时，provider 应回退到 active_providers[0]。
func TestLoadDefaultLLMSelectionFromFile_FlatFormActiveProviderFallback(t *testing.T) {
	selectionClearEnv(t, DefaultConfigEnvName, DefaultConfigPathEnv)

	path := selectionWriteConfig(t, `
environments:
  production:
    active_providers: ["ollama_local", "deepseek"]
    default_model: "unlisted-model"
`)

	selection, err := LoadDefaultLLMSelectionFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selection.Provider != "ollama_local" || selection.Model != "unlisted-model" {
		t.Fatalf("active_providers fallback failed, got %q/%q", selection.Provider, selection.Model)
	}
	if selection.Source != "environments.production" {
		t.Fatalf("unexpected source, got %q", selection.Source)
	}
}

func TestLoadDefaultLLMSelectionFromFile_FlatFormCloudModel(t *testing.T) {
	selectionClearEnv(t, DefaultConfigEnvName, DefaultConfigPathEnv)

	path := selectionWriteConfig(t, `
models:
  cloud_models:
    - name: "deepseek-chat"
      provider: "deepseek"
environments:
  production:
    active_providers: ["deepseek"]
    default_model: "deepseek-chat"
`)

	selection, err := LoadDefaultLLMSelectionFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selection.Provider != "deepseek" || selection.Model != "deepseek-chat" {
		t.Fatalf("cloud model provider lookup failed, got %q/%q", selection.Provider, selection.Model)
	}
	if selection.Source != "environments.production" {
		t.Fatalf("unexpected source, got %q", selection.Source)
	}
}

func TestLoadDefaultLLMSelectionFromFile_EnvProfile(t *testing.T) {
	selectionClearEnv(t, DefaultConfigEnvName, DefaultConfigPathEnv)

	path := selectionWriteConfig(t, `
models:
  local_models:
    - name: "codeqwen:7b"
      provider: "ollama_local"
environments:
  development:
    active_providers: ["ollama_local"]
    default_model: "codeqwen:7b"
  production:
    active_providers: ["deepseek"]
    default_model: "deepseek-chat"
`)

	t.Setenv(DefaultConfigEnvName, "development")

	selection, err := LoadDefaultLLMSelectionFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selection.Provider != "ollama_local" || selection.Model != "codeqwen:7b" {
		t.Fatalf("LLM_CONFIG_ENV profile not honored, got %q/%q", selection.Provider, selection.Model)
	}
	if selection.Source != "environments.development" {
		t.Fatalf("unexpected source, got %q", selection.Source)
	}
}

func TestLoadDefaultLLMSelectionFromFile_NoSelection(t *testing.T) {
	selectionClearEnv(t, DefaultConfigEnvName, DefaultConfigPathEnv)

	// 既没有 llm: 段，也没有 environments 段 => 应返回零值，由调用方回退到内置默认值。
	path := selectionWriteConfig(t, `
providers:
  ollama_local:
    priority: 1
`)

	selection, err := LoadDefaultLLMSelectionFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selection.Provider != "" || selection.Model != "" {
		t.Fatalf("expected empty selection, got %q/%q", selection.Provider, selection.Model)
	}
}

func TestLoadDefaultLLMSelectionFromFile_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.yaml")
	if _, err := LoadDefaultLLMSelectionFromFile(path); err == nil {
		t.Fatal("expected error for missing config file")
	}
}

func TestResolveDefaultConfigPath_EnvOverride(t *testing.T) {
	selectionClearEnv(t, DefaultConfigPathEnv)

	custom := selectionWriteConfig(t, "providers: {}\n")
	t.Setenv(DefaultConfigPathEnv, custom)

	path, err := ResolveDefaultConfigPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != custom {
		t.Fatalf("LLM_CONFIG_PATH override not honored, got %q want %q", path, custom)
	}
}
