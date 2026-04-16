package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/YuujiKamura/photo-ai-go/internal/ai"
	"github.com/YuujiKamura/photo-ai-go/internal/analyzer"
	"github.com/YuujiKamura/photo-ai-go/internal/extrapolator"
	"github.com/YuujiKamura/photo-ai-go/internal/matching"
	"github.com/YuujiKamura/photo-ai-go/internal/ocr"
	"github.com/YuujiKamura/photo-ai-go/internal/rules"
	"github.com/YuujiKamura/photo-ai-go/pkg/engine"
	"github.com/YuujiKamura/photo-ai-go/pkg/tagger"
	"github.com/YuujiKamura/photo-ai-go/pkg/utils/console"
)

// imageExtensions is the set of supported photo file extensions (lower-cased).
var imageExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".heic": true,
}

// ImageInfo holds basic file metadata before AI processing.
type ImageInfo struct {
	Path     string
	FileName string
	Date     string
}

// Result mirrors the Rust AnalysisResult for interop.
type Result struct {
	FileName           string   `json:"fileName"`
	FilePath           string   `json:"filePath"`
	Date               string   `json:"date"`
	WorkType           string   `json:"workType"`
	Variety            string   `json:"variety"`
	Subphase           string   `json:"subphase"`
	Station            string   `json:"station"`
	Remarks            string   `json:"remarks"`
	RemarksCandidates  []string `json:"remarksCandidates"`
	Description        string   `json:"description"`
	HasBoard           bool     `json:"hasBoard"`
	DetectedText       string   `json:"detectedText"`
	Measurements       string   `json:"measurements"`
	PhotoCategory      string   `json:"photoCategory"`
	Reasoning          string   `json:"reasoning"`
	FocusTarget        string   `json:"focusTarget"`
	Group              uint32   `json:"group"`
	WorkTypeCandidates []string `json:"workTypeCandidates,omitempty"`
}

// Config for Run.
type Config struct {
	Folder          string
	BatchSize       int
	WorkType        string
	PhotoType       string
	Variety         string
	Station         string
	Recursive       bool
	IncludeAll      bool
	PayPerUse       bool
	Limit           int
	UseCache        bool
	Extrapolator    interface {
		Lookup(currentDir, fileName, detectedText, description string) (extrapolator.SnapshotRecord, bool)
	}
	FolderRules     []rules.FolderRule
	AIClient        *ai.Client
	Matcher         *matching.Matcher
	HierarchyMaster *matching.HierarchyMaster
	ProcessImageFn  func(engine.ImageConfig) (engine.ImageResult, error)
	PairAfterFolder string
}

func Run(ctx context.Context, cfg Config) ([]Result, error) {
	fmt.Fprintf(console.Stdout(), "[1] 写真をスキャン中...\n")
	images, err := ScanImages(ctx, cfg.Folder, cfg.Recursive, cfg.IncludeAll)
	if err != nil {
		return nil, err
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("no images found in %s", cfg.Folder)
	}
	if cfg.Limit > 0 && len(images) > cfg.Limit {
		images = images[:cfg.Limit]
	}
	fmt.Fprintf(console.Stdout(), "✓ %d枚の写真を検出\n\n", len(images))

	images = ExtractEXIFDates(ctx, images)

	fmt.Fprintln(console.Stdout(), "[2] photo-tagger実行中...")
	processImage := engine.ProcessImage
	if cfg.ProcessImageFn != nil {
		processImage = cfg.ProcessImageFn
	}
	imgResult, err := processImage(engine.ImageConfig{
		Folder:     cfg.Folder,
		BatchSize:  cfg.BatchSize,
		WorkType:   cfg.WorkType,
		PhotoType:  cfg.PhotoType,
		Variety:    cfg.Variety,
		Station:    cfg.Station,
		UseCache:   cfg.UseCache,
		Recursive:  cfg.Recursive,
		IncludeAll: cfg.IncludeAll,
		PayPerUse:  cfg.PayPerUse,
	})
	if err != nil {
		return nil, fmt.Errorf("ProcessImage: %w", err)
	}

	var photoData map[string]ai.RawImageData
	if imgResult.OutputJSON != "" {
		photoData, _ = ai.DecodeGroupsFile(imgResult.OutputJSON)
	}
	if photoData == nil {
		photoData = make(map[string]ai.RawImageData)
	}

	allRaw := make([]ai.RawImageData, 0, len(photoData))
	for _, rd := range photoData {
		allRaw = append(allRaw, rd)
	}
	batchWorkTypes := analyzer.DetectWorkTypes(allRaw)

	folderName := filepath.Base(cfg.Folder)
	results := buildResults(ctx, images, photoData, batchWorkTypes, cfg.Matcher, cfg.HierarchyMaster, folderName)

	if cfg.Station != "" {
		applyStation(results, cfg.Station)
	}
	normalizeStations(results)

	if len(cfg.FolderRules) > 0 {
		rulesResults := resultsToRulesSlice(results)
		rulesResults = rules.ApplyFolderRules(rulesResults, folderName, cfg.FolderRules)
		results = rulesSliceToResults(rulesResults)
	}

	propagateContext(results)
	fmt.Fprintf(console.Stdout(), "✓ マスタ照合完了（%d枚）\n\n", len(results))

	return results, nil
}

func buildResults(ctx context.Context, images []ImageInfo, photoData map[string]ai.RawImageData, batchWorkTypes []string, m *matching.Matcher, hm *matching.HierarchyMaster, folderName string) []Result {
	results := make([]Result, 0, len(images))
	for _, img := range images {
		r := Result{
			FileName: img.FileName,
			FilePath: img.Path,
			Date:     img.Date,
			WorkTypeCandidates: batchWorkTypes,
		}
		if rd, ok := photoData[img.FileName]; ok {
			r.HasBoard = rd.HasBoard
			r.DetectedText = rd.DetectedText
			r.Description = rd.SceneDescription
		}

		kvs := ocr.ExtractKV(r.DetectedText)
		for _, kv := range kvs {
			if kv.Key == "場所" || kv.Key == "測点" {
				r.Station = ocr.NormalizeStation(kv.Value)
				break
			}
		}

		if hm != nil {
			keywords := []string{r.Description}
			for _, kv := range kvs {
				if kv.Value != "" { keywords = append(keywords, kv.Value) }
			}
			candidates := hm.GetCandidateRows(batchWorkTypes, keywords)
			if len(candidates) > 50 { candidates = candidates[:50] }

			if row, err := tagger.MapToMaster(ctx, r.FilePath, r.DetectedText, r.Description, candidates); err == nil && row != nil {
				r.WorkType = row.WorkType
				r.Variety = row.Variety
				r.Subphase = row.Subphase
				r.Remarks = row.Remarks
				r.PhotoCategory = row.PhotoType
			}
		}
		results = append(results, r)
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Date != results[j].Date { return results[i].Date < results[j].Date }
		return results[i].FileName < results[j].FileName
	})
	return results
}

func ScanImages(ctx context.Context, folder string, recursive, includeAll bool) ([]ImageInfo, error) {
	var images []ImageInfo
	err := filepath.Walk(folder, func(path string, info os.FileInfo, err error) error {
		if err != nil { return err }
		if !recursive && filepath.Dir(path) != folder {
			if info.IsDir() { return nil }
			return nil
		}
		if info.IsDir() {
			if !includeAll && isExcludedFolder(path) { return filepath.SkipDir }
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if includeAll || imageExtensions[ext] {
			images = append(images, ImageInfo{Path: path, FileName: info.Name()})
		}
		return nil
	})
	return images, err
}

func isExcludedFolder(path string) bool {
	normalized := filepath.ToSlash(path)
	for _, part := range strings.Split(normalized, "/") {
		p := strings.ToLower(part)
		if p == "非使用" || p == "hisiyou" || p == "trash" {
			return true
		}
		if len(p) > 1 && strings.HasPrefix(p, ".") {
			return true
		}
	}
	return false
}

func ExtractEXIFDates(ctx context.Context, images []ImageInfo) []ImageInfo {
	// Simplified for now, just uses file mod time if DLL not called
	for i := range images {
		if info, err := os.Stat(images[i].Path); err == nil {
			images[i].Date = info.ModTime().Format("2006-01-02 15:04:05")
		}
	}
	return images
}

func propagateContext(results []Result) {
	if len(results) < 3 { return }
	for i := 1; i < len(results)-1; i++ {
		r := &results[i]
		if r.WorkType == "" {
			prev, next := results[i-1], results[i+1]
			if prev.WorkType != "" && prev.WorkType == next.WorkType {
				r.WorkType = prev.WorkType
				if r.Variety == "" && prev.Variety == next.Variety {
					r.Variety = prev.Variety
				}
			}
		}
	}
}

func applyStation(results []Result, station string) {
	for i := range results {
		r := &results[i]
		if r.PhotoCategory == "安全管理" || r.PhotoCategory == "品質管理" {
			r.Station = dateToMonthDay(r.Date)
		} else {
			r.Station = station
		}
	}
}

func dateToMonthDay(date string) string {
	return date // simplified
}

func normalizeStations(results []Result) {
	for i := range results {
		results[i].Station = strings.TrimSpace(results[i].Station)
	}
}

func resultsToRulesSlice(src []Result) []rules.AnalysisResult {
	out := make([]rules.AnalysisResult, len(src))
	for i, r := range src {
		out[i] = rules.AnalysisResult{
			FileName: r.FileName,
			FilePath: r.FilePath,
			WorkType: r.WorkType,
			Variety:  r.Variety,
			Subphase: r.Subphase,
			Remarks:  r.Remarks,
			PhotoCategory: r.PhotoCategory,
		}
	}
	return out
}

func rulesSliceToResults(src []rules.AnalysisResult) []Result {
	out := make([]Result, len(src))
	for i, r := range src {
		out[i] = Result{
			FileName: r.FileName,
			FilePath: r.FilePath,
			WorkType: r.WorkType,
			Variety:  r.Variety,
			Subphase: r.Subphase,
			Remarks:  r.Remarks,
			PhotoCategory: r.PhotoCategory,
		}
	}
	return out
}
