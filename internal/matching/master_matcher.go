package matching

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/YuujiKamura/photo-ai-go/internal/ocr"
)

// ---- scoring constants (mirrors Rust) ----

const (
	focusTargetBoostMultiplier = 3
	searchPatternPriorityScore = 1000
)

// HierarchyRow mirrors Rust's HierarchyRow / CsvRow.
type HierarchyRow struct {
	PhotoDivision  string // 費目
	PhotoType      string // 写真区分
	WorkType       string // 工種
	Variety        string // 種別
	Subphase       string // 細別
	Remarks        string // 備考 / 撮影内容
	SearchPatterns string // 検索パターン (|–delimited)
}

// HierarchyMaster holds all rows from one or more master CSVs.
type HierarchyMaster struct {
	rows []HierarchyRow
}

// GetCandidateRows returns a subset of the master rows that are potentially relevant.
func (m *HierarchyMaster) GetCandidateRows(workTypes []string, keywords []string) []HierarchyRow {
	var candidates []HierarchyRow
	seen := make(map[string]bool)

	add := func(r HierarchyRow) {
		key := fmt.Sprintf("%s|%s|%s|%s", r.WorkType, r.Variety, r.Subphase, r.Remarks)
		if !seen[key] {
			candidates = append(candidates, r)
			seen[key] = true
		}
	}

	wtMap := make(map[string]bool)
	for _, wt := range workTypes {
		if wt != "" {
			wtMap[wt] = true
		}
	}

	for _, r := range m.rows {
		// Include if WorkType matches, or if it's a common entry (empty WorkType)
		if r.WorkType == "" || wtMap[r.WorkType] {
			add(r)
			continue
		}

		// Include if any keyword matches the row context
		for _, kw := range keywords {
			kw = strings.TrimSpace(kw)
			if kw == "" {
				continue
			}
			if strings.Contains(r.Remarks, kw) || strings.Contains(r.SearchPatterns, kw) {
				add(r)
				break
			}
		}
	}
	return candidates
}

func NewHierarchyMasterFromCSVString(content string) (*HierarchyMaster, error) {
	rows, err := parseHierarchyCSV(strings.NewReader(content))
	if err != nil {
		return nil, err
	}
	return &HierarchyMaster{rows: rows}, nil
}

func NewHierarchyMasterFromCSVFile(path string) (*HierarchyMaster, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rows, err := parseHierarchyCSV(f)
	if err != nil {
		return nil, err
	}
	return &HierarchyMaster{rows: rows}, nil
}

func NewHierarchyMasterFromBundleFiltered(dir string, targetWorkType string) (*HierarchyMaster, error) {
	byWorkType := filepath.Join(dir, "by_work_type")
	files := []string{"共通.csv", "安全管理.csv"}
	if targetWorkType != "" && targetWorkType != "共通" && targetWorkType != "安全管理" {
		files = append(files, targetWorkType+".csv")
	}

	var allRows []HierarchyRow
	for _, file := range files {
		csvPath := filepath.Join(byWorkType, file)
		if _, err := os.Stat(csvPath); err != nil {
			continue
		}
		f, err := os.Open(csvPath)
		if err != nil {
			continue
		}
		rows, err := parseHierarchyCSV(f)
		f.Close()
		if err == nil {
			allRows = append(allRows, rows...)
		}
	}
	if len(allRows) == 0 {
		return nil, fmt.Errorf("no master data found in %s", byWorkType)
	}
	return &HierarchyMaster{rows: allRows}, nil
}

func NewHierarchyMasterFromBundle(dir string) (*HierarchyMaster, error) {
	var files []string
	manifestPath := filepath.Join(dir, "manifest.json")
	if data, err := os.ReadFile(manifestPath); err == nil {
		var manifest struct {
			Files []string `json:"files"`
		}
		if err := json.Unmarshal(data, &manifest); err == nil {
			files = manifest.Files
		}
	}
	if len(files) == 0 {
		byWorkType := filepath.Join(dir, "by_work_type")
		if info, err := os.Stat(byWorkType); err == nil && info.IsDir() {
			entries, _ := os.ReadDir(byWorkType)
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".csv") {
					files = append(files, filepath.Join("by_work_type", e.Name()))
				}
			}
		}
	}
	var allRows []HierarchyRow
	for _, file := range files {
		csvPath := filepath.Join(dir, file)
		f, err := os.Open(csvPath)
		if err != nil {
			continue
		}
		rows, err := parseHierarchyCSV(f)
		f.Close()
		if err == nil {
			allRows = append(allRows, rows...)
		}
	}
	return &HierarchyMaster{rows: allRows}, nil
}

func normalizeWorkType(workType string) string {
	if strings.HasSuffix(workType, "補修工") {
		return strings.TrimSuffix(workType, "補修工") + "工"
	}
	return workType
}

func extractVarietyFromFolder(master *HierarchyMaster, folderName string) string {
	varieties := make(map[string]struct{})
	for _, r := range master.rows {
		if r.Variety != "" {
			varieties[r.Variety] = struct{}{}
		}
	}
	var bestVariety string
	var maxSim float64
	for v := range varieties {
		sim := CalculateSimilarity(folderName, v)
		if sim > maxSim && sim >= 0.2 {
			maxSim = sim
			bestVariety = v
		}
	}
	return bestVariety
}

func extractMatchKeywords(master *HierarchyMaster, detectedTexts []string, folderName string) ([]string, string, string) {
	var keywords []string
	var wtHint, vHint string
	for _, text := range detectedTexts {
		for _, kv := range ocr.ExtractKV(text) {
			k, v := kv.Key, kv.Value
			switch k {
			case "工種":
				if v != "" {
					wtHint = normalizeWorkType(v)
				}
			case "工事名", "車番", "車両番号", "場所", "測点":
			case "":
				if v != "" {
					keywords = append(keywords, v)
				}
			default:
				keywords = append(keywords, k)
				if v != "" {
					keywords = append(keywords, v)
				}
			}
		}
	}
	if vHint == "" {
		for _, kw := range keywords {
			for _, r := range master.rows {
				if r.Variety == kw || r.Subphase == kw {
					vHint = kw
					break
				}
			}
			if vHint != "" {
				break
			}
		}
	}
	genericNames := map[string]struct{}{
		"施工状況": {}, "品質管理": {}, "出来形管理": {}, "安全管理": {}, "使用材料": {}, "完成写真": {}, "着手前": {},
	}
	for _, token := range strings.FieldsFunc(folderName, func(r rune) bool {
		return r == '_' || r == '　' || r == ' ' || r == '・' || r == '.' || r == '※' || r == '(' || r == ')'
	}) {
		t := strings.TrimSpace(token)
		if utf8.RuneCountInString(t) >= 2 {
			if _, generic := genericNames[t]; !generic {
				keywords = append(keywords, t)
			}
		}
	}
	if vHint == "" && folderName != "" {
		vHint = extractVarietyFromFolder(master, folderName)
	}
	return keywords, wtHint, vHint
}

func scoreCandidates(master *HierarchyMaster, keywords []string, workTypeHint, varietyHint, focusTarget string) []struct {
	row   HierarchyRow
	score int
} {
	var candidates []HierarchyRow
	for _, r := range master.rows {
		if workTypeHint != "" && r.WorkType != workTypeHint && r.WorkType != "" {
			continue
		}
		if varietyHint != "" && r.Variety != varietyHint && r.Subphase != varietyHint && !strings.Contains(r.Variety, varietyHint) && !strings.Contains(varietyHint, r.Variety) {
			continue
		}
		candidates = append(candidates, r)
	}
	var scored []struct {
		row   HierarchyRow
		score int
	}
	for _, r := range candidates {
		if r.SearchPatterns != "" {
			patterns := strings.Split(r.SearchPatterns, "|")
			maxPScore := 0
			for _, kw := range keywords {
				for _, p := range patterns {
					if strings.Contains(kw, p) || strings.Contains(p, kw) {
						maxPScore = searchPatternPriorityScore
						break
					}
				}
				if maxPScore > 0 {
					break
				}
			}
			if maxPScore > 0 {
				scored = append(scored, struct {
					row   HierarchyRow
					score int
				}{r, maxPScore})
				continue
			}
		}
		kwScore := 0.0
		for _, kw := range keywords {
			kwScore += CalculateSimilarity(kw, r.Remarks)
		}
		ftBoost := 0.0
		if focusTarget != "" {
			ftBoost = CalculateSimilarity(focusTarget, r.Remarks) * focusTargetBoostMultiplier
		}
		total := int((kwScore + ftBoost) * 100)
		if total > 0 {
			scored = append(scored, struct {
				row   HierarchyRow
				score int
			}{r, total})
		}
	}
	return scored
}

func MatchMasterFromDetectedTexts(master *HierarchyMaster, detectedTexts []string, folderName, focusTarget string) *HierarchyRow {
	keywords, wtHint, vHint := extractMatchKeywords(master, detectedTexts, folderName)
	scored := scoreCandidates(master, keywords, wtHint, vHint, focusTarget)
	if len(scored) == 0 {
		return nil
	}
	sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	return &scored[0].row
}

func parseHierarchyCSV(r io.Reader) ([]HierarchyRow, error) {
	cr := csv.NewReader(r)
	cr.LazyQuotes, cr.TrimLeadingSpace = true, true
	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("csv header: %w", err)
	}
	idx := map[string]int{}
	for i, h := range header {
		idx[strings.TrimSpace(h)] = i
	}
	mustCol := func(names ...string) int {
		for _, n := range names {
			if i, ok := idx[n]; ok {
				return i
			}
		}
		return -1
	}
	colPD, colPT, colWT, colV, colS, colR, colSP := mustCol("費目"), mustCol("写真区分"), mustCol("工種"), mustCol("種別"), mustCol("細別"), mustCol("備考", "撮影内容"), mustCol("検索パターン")
	if colWT == -1 || colR == -1 {
		return nil, fmt.Errorf("essential columns missing")
	}
	safeGet := func(rec []string, col int) string {
		if col < 0 || col >= len(rec) {
			return ""
		}
		return strings.TrimSpace(rec[col])
	}
	var rows []HierarchyRow
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		rows = append(rows, HierarchyRow{
			PhotoDivision: safeGet(rec, colPD), PhotoType: safeGet(rec, colPT), WorkType: safeGet(rec, colWT),
			Variety: safeGet(rec, colV), Subphase: safeGet(rec, colS),
			Remarks: strings.ReplaceAll(safeGet(rec, colR), `\n`, "\n"), SearchPatterns: safeGet(rec, colSP),
		})
	}
	return rows, nil
}
