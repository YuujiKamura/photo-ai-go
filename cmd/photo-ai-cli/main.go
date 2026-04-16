package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/YuujiKamura/photo-ai-go/internal/export"
	"github.com/YuujiKamura/photo-ai-go/internal/matching"
	"github.com/YuujiKamura/photo-ai-go/internal/pipeline"
	"github.com/YuujiKamura/photo-ai-go/pkg/engine"
	"github.com/YuujiKamura/photo-ai-go/pkg/utils/console"
)

func init() {
	// Wrap stdout/stderr for CP932 auto-conversion on Windows terminals
	rootCmd.SetOut(console.Stdout())
	rootCmd.SetErr(console.Stderr())
}

type analyzeFlagSet struct {
	batchSize   int
	workType    string
	photoType   string
	variety     string
	station     string
	useCache    bool
	recursive   bool
	includeAll  bool
	folderRules string
	payPerUse   bool
}

var gitCommit = "unknown"
var buildDate = "unknown"

func main() {
	// Apply global console wrapping for Windows encoding
	out := console.Stdout()
	errOut := console.Stderr()
	
	fmt.Fprintf(out, "photo-ai-cli version (commit:%s buildDate:%s)\n\n", gitCommit, buildDate)
	
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(errOut, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "photo-ai-cli",
	Short: "工事写真AI解析・写真台帳生成ツール (CLI 専用)",
	Long: `photo-ai-cli — Construction photo AI analysis and photo ledger generation tool.

CLI-only entrypoint. It supports local analysis and pairing flows without the Web UI server.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		engine.GitCommit = gitCommit
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(analyzeCmd)
	rootCmd.AddCommand(pairCmd)

	analyzeCmd.Flags().StringP("output", "o", "", "Output JSON file (default: <folder>/result.json)")
	analyzeCmd.Flags().IntP("batch-size", "b", 5, "Number of photos per AI batch")
	analyzeCmd.Flags().StringP("master", "m", "", "Work-type master CSV/JSON file")
	analyzeCmd.Flags().StringP("work-type", "w", "", "Work type override (1-step analysis mode)")
	analyzeCmd.Flags().StringP("photo-type", "t", "", "Photo type override (e.g. 使用機械)")
	analyzeCmd.Flags().String("variety", "", "Variety override")
	analyzeCmd.Flags().StringP("station", "s", "", "Station override (applied to all photos)")
	analyzeCmd.Flags().Bool("use-cache", false, "Skip re-analysis of already-cached photos")
	analyzeCmd.Flags().BoolP("recursive", "r", false, "Scan sub-folders recursively")
	analyzeCmd.Flags().Bool("include-all", false, "Include folders normally excluded (e.g. 非使用)")
	analyzeCmd.Flags().String("line-types", "", "Line-type list JSON file (区画線工)")
	analyzeCmd.Flags().String("folder-rules", "", "Folder-rule override JSON file")
	analyzeCmd.Flags().Bool("pay-per-use", false, "Use pay-per-use billing instead of subscription/resident mode")

	pairCmd.Flags().StringP("input", "i", "", "Input analysis result JSON (required)")
	pairCmd.Flags().StringP("output", "o", "", "Output paired JSON file")
}

var analyzeCmd = &cobra.Command{
	Use:   "analyze <folder>",
	Short: "写真フォルダを解析してJSONを出力",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		folder := args[0]

		outputFlag, _ := cmd.Flags().GetString("output")
		master, _ := cmd.Flags().GetString("master")

		batchSize, _ := cmd.Flags().GetInt("batch-size")
		workType, _ := cmd.Flags().GetString("work-type")
		photoType, _ := cmd.Flags().GetString("photo-type")
		variety, _ := cmd.Flags().GetString("variety")
		station, _ := cmd.Flags().GetString("station")
		useCache, _ := cmd.Flags().GetBool("use-cache")
		recursive, _ := cmd.Flags().GetBool("recursive")
		includeAll, _ := cmd.Flags().GetBool("include-all")
		folderRules, _ := cmd.Flags().GetString("folder-rules")
		payPerUse, _ := cmd.Flags().GetBool("pay-per-use")

		flags := analyzeFlagSet{
			batchSize:   batchSize,
			workType:    workType,
			photoType:   photoType,
			variety:     variety,
			station:     station,
			useCache:    useCache,
			recursive:   recursive,
			includeAll:  includeAll,
			folderRules: folderRules,
			payPerUse:   payPerUse,
		}

		destJSON := resolveOutputJSON(folder, outputFlag)

		fmt.Fprintln(console.Stderr(), "写真解析を開始しています...")

		return runAnalyzeDirect(folder, destJSON, master, flags)
	},
}

var pairCmd = &cobra.Command{
	Use:   "pair",
	Short: "施工前後の写真ペアリングを実行",
	RunE: func(cmd *cobra.Command, args []string) error {
		inputFlag, _ := cmd.Flags().GetString("input")
		outputFlag, _ := cmd.Flags().GetString("output")

		if inputFlag == "" {
			return fmt.Errorf("pair: --input is required")
		}

		fmt.Fprintln(console.Stderr(), "ペアリングを開始しています...")

		results, err := export.LoadResults(inputFlag)
		if err != nil {
			return fmt.Errorf("pair: load results: %w", err)
		}

		paired := runPairing(results)

		destPath := outputFlag
		if destPath == "" {
			dir := filepath.Dir(inputFlag)
			destPath = filepath.Join(dir, "paired.json")
		}

		data, err := json.MarshalIndent(paired, "", "  ")
		if err != nil {
			return fmt.Errorf("pair: marshal: %w", err)
		}
		if err := os.WriteFile(destPath, data, 0o644); err != nil {
			return fmt.Errorf("pair: write %s: %w", destPath, err)
		}

		fmt.Fprintf(console.Stderr(), "ペアリング完了: %dペア → %s\n", len(paired), destPath)
		return nil
	},
}

type PairedResult struct {
	Before engine.AnalysisResult `json:"before"`
	After  engine.AnalysisResult `json:"after"`
	Score  float64               `json:"score"`
}

func runPairing(results []engine.AnalysisResult) []PairedResult {
	var before, after []engine.AnalysisResult
	for _, r := range results {
		switch r.Subphase {
		case "施工前", "着手前":
			before = append(before, r)
		case "施工後", "竣工":
			after = append(after, r)
		}
	}

	var pairs []PairedResult
	limit := len(before)
	if len(after) < limit {
		limit = len(after)
	}
	for i := 0; i < limit; i++ {
		pairs = append(pairs, PairedResult{
			Before: before[i],
			After:  after[i],
			Score:  1.0,
		})
	}
	return pairs
}

func resolveOutputJSON(folder, outputFlag string) string {
	if outputFlag != "" {
		return outputFlag
	}
	return filepath.Join(folder, export.DefaultResultFileName)
}

func runAnalyzeDirect(folder, destJSON, master string, flags analyzeFlagSet) error {
	cfg := pipeline.Config{
		Folder:     folder,
		BatchSize:  flags.batchSize,
		WorkType:   flags.workType,
		PhotoType:  flags.photoType,
		Variety:    flags.variety,
		Station:    flags.station,
		UseCache:   flags.useCache,
		Recursive:  flags.recursive,
		IncludeAll: flags.includeAll,
		PayPerUse:  flags.payPerUse,
	}

	if master != "" {
		hm, err := matching.NewHierarchyMasterFromCSVFile(master)
		if err != nil {
			return fmt.Errorf("load master %s: %w", master, err)
		}
		cfg.HierarchyMaster = hm
	}

	results, err := pipeline.Run(context.Background(), cfg)
	if err != nil {
		return fmt.Errorf("run pipeline: %w", err)
	}

	engineResults := pipelineResultsToEngineResults(results)
	stampAnalysisMetadata(engineResults, analyzeMetadata{
		MasterPath: master,
		WorkType:   flags.workType,
		PhotoType:  flags.photoType,
		Variety:    flags.variety,
		PayPerUse:  flags.payPerUse,
	})

	data, err := json.MarshalIndent(engineResults, "", "  ")
	if err != nil {
		return fmt.Errorf("serialize final result: %w", err)
	}
	if err := os.WriteFile(destJSON, data, 0o644); err != nil {
		return fmt.Errorf("save final result: %w", err)
	}
	fmt.Fprintf(console.Stderr(), "最終結果を保存: %s\n", destJSON)
	return nil
}

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
			Description:   r.Description,
			HasBoard:      r.HasBoard,
			DetectedText:  r.DetectedText,
			Measurements:  r.Measurements,
			PhotoCategory: r.PhotoCategory,
			Reasoning:     r.Reasoning,
			FocusTarget:   r.FocusTarget,
			Group:         r.Group,
		}
	}
	return out
}

type analyzeMetadata struct {
	MasterPath string
	WorkType   string
	PhotoType  string
	Variety    string
	PayPerUse  bool
}

func stampAnalysisMetadata(results []engine.AnalysisResult, meta analyzeMetadata) {
	timestamp := time.Now().Format("2006-01-02 15:04:05 -0700")
	masterSelection := "all"
	if meta.MasterPath != "" {
		masterSelection = "single"
	}

	for i := range results {
		results[i].AnalysisTimestamp = timestamp
		results[i].AnalysisProvider = "auto"
		results[i].AnalysisCommit = gitCommit
		results[i].AnalysisMasterSelection = masterSelection
		results[i].AnalysisMasterPath = meta.MasterPath
		results[i].AnalysisScopeWorkType = meta.WorkType
		results[i].AnalysisScopePhotoType = meta.PhotoType
		results[i].AnalysisScopeVariety = meta.Variety
	}
}
