package tagger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/YuujiKamura/resident-agent/pkg/api"
	"github.com/YuujiKamura/photo-ai-go/pkg/utils/console"
)

func init() {
	// 1. Set defaults to 'auto' for intelligent routing and maximum stability
	viper.SetDefault("model", "auto")
	
	// 2. Read from config file
	viper.SetConfigName(".photo-ai")
	viper.SetConfigType("json")
	viper.AddConfigPath(".")
	_ = viper.ReadInConfig()

	// 3. Bind environment variables (e.g. PHOTO_AI_MODEL)
	viper.SetEnvPrefix("PHOTO_AI")
	viper.AutomaticEnv()
}

// GetDefaultModel resolves the model name via Viper (ENV > CONFIG > DEFAULT)
func GetDefaultModel() string {
	return viper.GetString("model")
}

// ClassifyConfig holds configuration for the Gemini classify call.
type ClassifyConfig struct {
	Model      string
	Vocabulary []string
	PayPerUse  bool
}

func (c *ClassifyConfig) model() string {
	if c.Model != "" {
		return c.Model
	}
	return GetDefaultModel()
}

// BatchResult holds items and profiling info for a single batch.
type BatchResult struct {
	Items     []GroupItem
	LatencyMs int64
	Mode      string
}

// ClassifyGroupBatch uses resident-agent as a library to classify a batch of images.
func ClassifyGroupBatch(ctx context.Context, images []string, cfg *ClassifyConfig) (BatchResult, error) {
	start := time.Now()

	opts := api.DefaultOptions().
		WithModel(cfg.model()).
		WithJSON()

	prompt := GroupPrompt(images, cfg.Vocabulary)
	fmt.Fprintf(console.Stdout(), "[debug] resident-agent prompt: %s\n", prompt)

	// Direct function call! No temp files, no sub-processes.
	rawResponse, err := api.Analyze(prompt, images, opts)
	if err != nil {
		return BatchResult{}, fmt.Errorf("api.Analyze failed: %w", err)
	}
	fmt.Fprintf(console.Stdout(), "[debug] resident-agent response raw:\n%s\n", rawResponse)

	items, err := parseItems(rawResponse)
	if err != nil {
		return BatchResult{}, err
	}

	return BatchResult{
		Items:     items,
		LatencyMs: time.Since(start).Milliseconds(),
		Mode:      "DIRECT_LIB_CALL",
	}, nil
}

func parseItems(rawText string) ([]GroupItem, error) {
	jsonStr, err := extractJSONArray(rawText)
	if err != nil {
		return nil, fmt.Errorf("no JSON array in response: %w\nraw: %s", err, rawText)
	}

	var items []GroupItem
	if err := json.Unmarshal([]byte(jsonStr), &items); err != nil {
		return nil, fmt.Errorf("failed to parse group JSON: %w\njson: %s", err, jsonStr)
	}

	return items, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// extractJSONArray finds the outermost JSON array in a string.
func extractJSONArray(s string) (string, error) {
	if idx := strings.Index(s, "```json"); idx >= 0 {
		start := idx + len("```json")
		rest := s[start:]
		if end := strings.Index(rest, "```"); end >= 0 {
			return strings.TrimSpace(rest[:end]), nil
		}
	}
	lastBracket := strings.LastIndex(s, "]")
	if lastBracket < 0 {
		return "", fmt.Errorf("no JSON array found")
	}
	for i := lastBracket; i >= 0; i-- {
		if s[i] == '[' {
			candidate := s[i : lastBracket+1]
			var v json.RawMessage
			if json.Unmarshal([]byte(candidate), &v) == nil {
				return candidate, nil
			}
		}
	}
	return "", fmt.Errorf("no valid JSON array found")
}
