package extrapolator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/YuujiKamura/photo-ai-go/internal/matching"
	"github.com/YuujiKamura/photo-ai-go/pkg/engine"
)

type SnapshotRecord struct {
	FileName     string
	DirName      string
	DetectedText string
	Description  string
	WorkType     string
	Variety      string
	Subphase     string
	Remarks      string
	IsGroundTruth bool
}

type Extrapolator struct {
	records []SnapshotRecord
}

func NewExtrapolator(searchDirs ...string) (*Extrapolator, error) {
	e := &Extrapolator{}
	
	for _, dir := range searchDirs {
		isGT := strings.Contains(strings.ToLower(dir), "ground-truth")
		count := 0
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(strings.ToLower(info.Name()), ".json") {
				return nil
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}

			var results []engine.AnalysisResult
			if err := json.Unmarshal(data, &results); err == nil {
				dirName := filepath.Base(filepath.Dir(path))
				if dirName == "json" { // ground-truth structure
					dirName = filepath.Base(filepath.Dir(filepath.Dir(path)))
				}
				for _, r := range results {
					if r.WorkType != "" {
						e.records = append(e.records, SnapshotRecord{
							FileName:      r.FileName,
							DirName:       dirName,
							DetectedText:  r.DetectedText,
							Description:   r.Description,
							WorkType:      r.WorkType,
							Variety:       r.Variety,
							Subphase:      r.Subphase,
							Remarks:       r.Remarks,
							IsGroundTruth: isGT,
						})
						count++
					}
				}
			}
			return nil
		})
		if count > 0 {
			label := "snapshots"
			if isGT {
				label = "GROUND TRUTH"
			}
			fmt.Fprintf(os.Stderr, "[extrapolator] %s から %d 件をロードしました\n", label, count)
		}
	}
	return e, nil
}

// Lookup finds the best match. 
func (e *Extrapolator) Lookup(currentDir, fileName, detectedText, description string) (SnapshotRecord, bool) {
	var bestRec SnapshotRecord
	var maxScore float64

	// Pass 1: Identical filename in identical site (DirName fuzzy match)
	for _, rec := range e.records {
		if rec.FileName == fileName {
			dirSimilarity := matching.CalculateSimilarity(currentDir, rec.DirName)
			if dirSimilarity > 0.6 {
				return rec, true // Strong match by filename + site context
			}
		}
	}

	// Pass 2: High similarity match
	const threshold = 0.6 // Stricter threshold
	for _, rec := range e.records {
		scoreOCR := matching.CalculateSimilarity(detectedText, rec.DetectedText)
		scoreDesc := matching.CalculateSimilarity(description, rec.Description)

		score := scoreOCR
		if scoreDesc > score {
			score = scoreDesc * 0.7
		}
		
		// Site context boost
		if matching.CalculateSimilarity(currentDir, rec.DirName) > 0.5 {
			score += 0.2
		}

		if rec.IsGroundTruth {
			score += 0.1
		}

		if score > maxScore && score >= threshold {
			maxScore = score
			bestRec = rec
		}
	}

	if maxScore > 0 {
		return bestRec, true
	}
	return SnapshotRecord{}, false
}
