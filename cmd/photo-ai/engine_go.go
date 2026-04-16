// Package main — engine_go.go
// runGoEngine contains the Go-native pipeline path for the analyze command.
// It calls internal/pipeline.Run and serialises []pipeline.Result to the same
// result.json schema that the Rust path produces.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/YuujiKamura/photo-ai-go/internal/master"
	"github.com/YuujiKamura/photo-ai-go/internal/matching"
	"github.com/YuujiKamura/photo-ai-go/internal/pipeline"
	"github.com/YuujiKamura/photo-ai-go/pkg/engine"
)

// runGoEngine executes the Go-native pipeline and writes result.json.
// The output schema mirrors engine.AnalysisResult so downstream consumers
// (export pdf/excel, pair) can work with either engine's output unchanged.
func runGoEngine(folder, destJSON, masterPath string, flags analyzeFlagSet) error {
	// Initialize extrapolator from snapshots AND ground-truth
	// ex, _ := extrapolator.NewExtrapolator(
	// 	`C:\Users\yuuji\analysis-snapshots`,
	// 	`C:\Users\yuuji\photo-ai-ground-truth`,
	// )

	cfg := pipeline.Config{
		Folder:       folder,
		BatchSize:    flags.batchSize,
		WorkType:     flags.workType,
		PhotoType:    flags.photoType,
		Variety:      flags.variety,
		Station:      flags.station,
		UseCache:     flags.useCache,
		Recursive:    flags.recursive,
		IncludeAll:   flags.includeAll,
		Limit:        flags.limit,
		PayPerUse:    flags.payPerUse,
		Extrapolator: nil, // ex,
	}

	// Resolve and load master data
	resolvedMaster, err := master.ResolveMasterDir(masterPath)
	if err == nil {
		var hm *matching.HierarchyMaster
		info, _ := os.Stat(resolvedMaster)
		
		if info.IsDir() {
			// If user specified a work type, only load relevant CSVs from the bundle
			if flags.workType != "" {
				fmt.Fprintf(os.Stderr, "[go engine] 工種 %q に関連するマスタのみをロードします\n", flags.workType)
				hm, err = matching.NewHierarchyMasterFromBundleFiltered(resolvedMaster, flags.workType)
			} else {
				fmt.Fprintf(os.Stderr, "[go engine] マスタディレクトリをロード中: %s\n", resolvedMaster)
				hm, err = matching.NewHierarchyMasterFromBundle(resolvedMaster)
			}
		} else {
			fmt.Fprintf(os.Stderr, "[go engine] マスタCSVをロード中: %s\n", resolvedMaster)
			hm, err = matching.NewHierarchyMasterFromCSVFile(resolvedMaster)
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "[warning] マスタのロードに失敗しました: %v\n", err)
		} else {
			cfg.HierarchyMaster = hm
		}
	} else {
		fmt.Fprintf(os.Stderr, "[warning] 工種マスタが見つかりません。マッチングをスキップします。\n")
	}

	results, err := pipeline.Run(context.Background(), cfg)
	if err != nil {
		return fmt.Errorf("go engine: %w", err)
	}

	// Convert pipeline.Result → engine.AnalysisResult so the JSON output matches
	// the established result.json schema.
	engineResults := pipelineResultsToEngineResults(results)

	data, err := json.MarshalIndent(engineResults, "", "  ")
	if err != nil {
		return fmt.Errorf("go engine: marshal results: %w", err)
	}
	if err := os.WriteFile(destJSON, data, 0o644); err != nil {
		return fmt.Errorf("go engine: write %s: %w", destJSON, err)
	}

	printAnalysisProfile(engineResults)
	fmt.Fprintf(os.Stderr, "[go engine] 完了: %d枚 → %s\n", len(engineResults), destJSON)
	return nil
}

// printAnalysisProfile calculates and prints the profiling summary to stderr.
func printAnalysisProfile(results []engine.AnalysisResult) {
	var totalMs int64
	var count int
	for _, r := range results {
		if r.AnalysisLatencyMs > 0 {
			totalMs += r.AnalysisLatencyMs
			count++
		}
	}
	if count == 0 {
		return
	}
	avgMs := totalMs / int64(count)
	fmt.Fprintf(os.Stderr, "\n--- 解析プロファイル ---\n")
	fmt.Fprintf(os.Stderr, "対象枚数: %d 枚\n", count)
	fmt.Fprintf(os.Stderr, "合計時間: %.2f 秒\n", float64(totalMs)/1000.0)
	fmt.Fprintf(os.Stderr, "平均時間: %d ms/枚\n", avgMs)
	fmt.Fprintf(os.Stderr, "------------------------\n\n")
}

// pipelineResultsToEngineResults converts the Go-native pipeline output to the
// canonical engine.AnalysisResult schema used by result.json.
// Fields absent from pipeline.Result are left at their zero values; they will
// be populated once Stream H completes the pipeline port (see repo issue #165).
func pipelineResultsToEngineResults(src []pipeline.Result) []engine.AnalysisResult {
	out := make([]engine.AnalysisResult, len(src))
	for i, r := range src {
		out[i] = engine.AnalysisResult{
			FileName:      r.FileName,
			FilePath:      r.FilePath,
			Date:          r.Date,
			WorkType:      r.WorkType,
			Variety:       r.Variety,
			Subphase:      r.Subphase,
			Station:       r.Station,
			Remarks:       r.Remarks,
			RemarksCandidates: []string{},
			Description:   r.Description,
			HasBoard:      r.HasBoard,
			DetectedText:  r.DetectedText,
			Measurements:  r.Measurements,
			PhotoCategory: r.PhotoCategory,
			Reasoning:     r.Reasoning,
			FocusTarget:   r.FocusTarget,
			Group:         r.Group,
			LabelOverrides: map[string]string{},
		}
	}
	return out
}
