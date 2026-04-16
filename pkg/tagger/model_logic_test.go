package tagger

import (
	"testing"
	"github.com/spf13/viper"
)

func TestModelResolution(t *testing.T) {
	viper.Reset()
	tests := []struct {
		name     string
		input    string
		viperVal string
		expected string
	}{
		{
			name:     "Explicit model",
			input:    "gemini-1.5-flash",
			expected: "gemini-1.5-flash",
		},
		{
			name:     "Auto means empty (to omit -m flag)",
			input:    "auto",
			expected: "",
		},
		{
			name:     "Viper default auto means empty",
			input:    "",
			viperVal: "auto",
			expected: "",
		},
		{
			name:     "Empty is empty",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			if tt.viperVal != "" {
				viper.Set("model", tt.viperVal)
			}
			cfg := &ClassifyConfig{Model: tt.input}
			got := cfg.model()
			if got != tt.expected {
				t.Errorf("model() = %q, want %q", got, tt.expected)
			}
		})
	}
}
