package client

import (
	appconfig "late/internal/config"
	"testing"
)

func TestResolveConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *appconfig.Config
		env     map[string]string
		present map[string]bool
		want    Config
	}{
		{
			name: "env only",
			env: map[string]string{
				"OPENAI_BASE_URL": "https://env.example",
				"OPENAI_API_KEY":  "env-key",
				"OPENAI_MODEL":    "env-model",
			},
			present: map[string]bool{
				"OPENAI_BASE_URL": true,
				"OPENAI_API_KEY":  true,
				"OPENAI_MODEL":    true,
			},
			want: Config{BaseURL: "https://env.example", APIKey: "env-key", Model: "env-model"},
		},
		{
			name: "config only",
			cfg: &appconfig.Config{
				OpenAIBaseURL: "https://config.example",
				OpenAIAPIKey:  "config-key",
				OpenAIModel:   "config-model",
			},
			want: Config{BaseURL: "https://config.example", APIKey: "config-key", Model: "config-model"},
		},
		{
			name: "env wins over config",
			cfg: &appconfig.Config{
				OpenAIBaseURL: "https://config.example",
				OpenAIAPIKey:  "config-key",
				OpenAIModel:   "config-model",
			},
			env: map[string]string{
				"OPENAI_BASE_URL": "https://env.example",
				"OPENAI_API_KEY":  "env-key",
				"OPENAI_MODEL":    "env-model",
			},
			present: map[string]bool{
				"OPENAI_BASE_URL": true,
				"OPENAI_API_KEY":  true,
				"OPENAI_MODEL":    true,
			},
			want: Config{BaseURL: "https://env.example", APIKey: "env-key", Model: "env-model"},
		},
		{
			name: "none set uses default URL",
			want: Config{BaseURL: DefaultBaseURL},
		},
		{
			name: "empty env falls back to config",
			cfg: &appconfig.Config{
				OpenAIBaseURL: "https://config.example",
				OpenAIAPIKey:  "config-key",
				OpenAIModel:   "config-model",
			},
			env: map[string]string{
				"OPENAI_BASE_URL": "",
				"OPENAI_API_KEY":  "",
				"OPENAI_MODEL":    "",
			},
			present: map[string]bool{
				"OPENAI_BASE_URL": true,
				"OPENAI_API_KEY":  true,
				"OPENAI_MODEL":    true,
			},
			want: Config{BaseURL: "https://config.example", APIKey: "config-key", Model: "config-model"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveConfigWithEnv(tt.cfg, func(key string) (string, bool) {
				value, ok := tt.env[key]
				if tt.present != nil {
					ok = tt.present[key]
				}
				return value, ok
			})

			if got.BaseURL != tt.want.BaseURL {
				t.Fatalf("BaseURL = %q, want %q", got.BaseURL, tt.want.BaseURL)
			}
			if got.APIKey != tt.want.APIKey {
				t.Fatalf("APIKey = %q, want %q", got.APIKey, tt.want.APIKey)
			}
			if got.Model != tt.want.Model {
				t.Fatalf("Model = %q, want %q", got.Model, tt.want.Model)
			}
		})
	}
}
