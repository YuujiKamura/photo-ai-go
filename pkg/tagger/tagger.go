package tagger

import (
	"context"
	"fmt"
	"path/filepath"
)

const defaultBatchSize = 10

// Config holds parameters for RunGrouping.
type Config struct {
	Folder     string
	BatchSize  int
	PayPerUse  bool
	APIKey     string
	Model      string
	Vocabulary []string
}

// Result is returned by RunGrouping.
type Result struct {
	PhotoCount int
	OutputJSON string
}

// RunGrouping scans a folder, classifies pending images via Gemini,
// assigns groups, and saves photo-groups.json.
func RunGrouping(ctx context.Context, cfg Config) (Result, error) {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = defaultBatchSize
	}

	records := LoadGroupRecords(cfg.Folder)

	images := CollectImagesFlat(cfg.Folder)

	captureTimes := collectCaptureTimes(images)

	if len(images) == 0 {
		return Result{PhotoCount: 0, OutputJSON: filepath.Join(cfg.Folder, "photo-groups.json")}, nil
	}

	forceReclassify := forceReclassifyEnabled()

	// Determine pending images
	var pending []string
	if forceReclassify {
		pending = images
	} else {
		for _, img := range images {
			fname := filepath.Base(img)
			if _, exists := records[fname]; !exists {
				pending = append(pending, img)
			}
		}
	}

	if len(pending) > 0 {
		classifyCfg := &ClassifyConfig{
			Model:      cfg.Model,
			Vocabulary: cfg.Vocabulary,
			PayPerUse:  cfg.PayPerUse,
		}

		for batchIdx := 0; batchIdx*cfg.BatchSize < len(pending); batchIdx++ {
			lo := batchIdx * cfg.BatchSize
			hi := lo + cfg.BatchSize
			if hi > len(pending) {
				hi = len(pending)
			}
			batch := pending[lo:hi]

			batchRes, err := ClassifyGroupBatch(ctx, batch, classifyCfg)
			if err != nil {
				return Result{}, fmt.Errorf("classify batch %d: %w", batchIdx+1, err)
			}

			avgLatency := batchRes.LatencyMs / int64(len(batch))
			if avgLatency == 0 {
				avgLatency = 1 // Ensure non-zero for debugging if it's too fast
			}

			for _, item := range batchRes.Items {
				records[item.File] = &GroupRecord{
					GroupCore:         item.GroupCore,
					Group:             0,
					AnalysisLatencyMs: avgLatency,
				}
			}
		}
	}

	applyCaptureTimes(records, captureTimes)
	assignGroups(records)

	if err := SaveGroupRecords(cfg.Folder, records); err != nil {
		return Result{}, fmt.Errorf("save group records: %w", err)
	}

	return Result{
		PhotoCount: len(records),
		OutputJSON: filepath.Join(cfg.Folder, "photo-groups.json"),
	}, nil
}
