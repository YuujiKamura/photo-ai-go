package matching

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseHierarchyCSV(t *testing.T) {
	content := "費目,写真区分,工種,種別,細別,備考,検索パターン\n" +
		"\"直接工事費\",\"施工状況写真\",\"舗装工\",\"舗装打換え工\",\"表層工\",\"舗設状況\",\"舗設|フィニッシャー\"\n" +
		"\"共通仮設費\",\"安全管理写真\",\"\",\"\",\"\",\"朝礼・KY活動実施状況\",\"朝礼|KY活動\""

	rows, err := parseHierarchyCSV(strings.NewReader(content))
	if err != nil {
		t.Fatalf("parseHierarchyCSV failed: %v", err)
	}

	if len(rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(rows))
	}

	if rows[1].Remarks != "朝礼・KY活動実施状況" {
		t.Errorf("expected remarks '朝礼・KY活動実施状況', got %q", rows[1].Remarks)
	}
}

func TestNewHierarchyMasterFromBundle(t *testing.T) {
	tmpDir := t.TempDir()
	byWorkType := filepath.Join(tmpDir, "by_work_type")
	os.MkdirAll(byWorkType, 0755)

	csvContent := "費目,写真区分,工種,種別,細別,備考,検索パターン\n\"A\",\"B\",\"C\",\"D\",\"E\",\"F\",\"G\""
	os.WriteFile(filepath.Join(byWorkType, "test.csv"), []byte(csvContent), 0644)

	// Test 1: Load without manifest (fallback to by_work_type scan)
	m, err := NewHierarchyMasterFromBundle(tmpDir)
	if err != nil {
		t.Fatalf("NewHierarchyMasterFromBundle failed: %v", err)
	}
	if len(m.rows) != 1 {
		t.Errorf("expected 1 row from scan, got %d", len(m.rows))
	}

	// Test 2: Load with manifest
	manifest := `{"files": ["by_work_type/test.csv"]}`
	os.WriteFile(filepath.Join(tmpDir, "manifest.json"), []byte(manifest), 0644)
	m2, err := NewHierarchyMasterFromBundle(tmpDir)
	if err != nil {
		t.Fatalf("NewHierarchyMasterFromBundle with manifest failed: %v", err)
	}
	if len(m2.rows) != 1 {
		t.Errorf("expected 1 row from manifest, got %d", len(m2.rows))
	}
}

func TestNewHierarchyMasterFromBundleFiltered(t *testing.T) {
	tmpDir := t.TempDir()
	byWorkType := filepath.Join(tmpDir, "by_work_type")
	os.MkdirAll(byWorkType, 0755)

	header := "費目,写真区分,工種,種別,細別,備考,検索パターン\n"
	os.WriteFile(filepath.Join(byWorkType, "共通.csv"), []byte(header+"\"共通\",\"\",\"\",\"\",\"\",\"共通項目\",\"\""), 0644)
	os.WriteFile(filepath.Join(byWorkType, "安全管理.csv"), []byte(header+"\"安全\",\"\",\"\",\"\",\"\",\"安全項目\",\"\""), 0644)
	os.WriteFile(filepath.Join(byWorkType, "舗装工.csv"), []byte(header+"\"舗装\",\"\",\"舗装工\",\"\",\"\",\"舗装項目\",\"\""), 0644)

	// Test: Filtered load should include 共通 + 安全管理 + target
	m, err := NewHierarchyMasterFromBundleFiltered(tmpDir, "舗装工")
	if err != nil {
		t.Fatalf("NewHierarchyMasterFromBundleFiltered failed: %v", err)
	}

	foundCommon := false
	foundSafety := false
	foundPavement := false
	for _, r := range m.rows {
		if r.PhotoDivision == "共通" { foundCommon = true }
		if r.PhotoDivision == "安全" { foundSafety = true }
		if r.PhotoDivision == "舗装" { foundPavement = true }
	}

	if !foundCommon || !foundSafety || !foundPavement {
		t.Errorf("missing expected files in filtered load: common=%v, safety=%v, pavement=%v", 
			foundCommon, foundSafety, foundPavement)
	}
}

func TestGetCandidateRows(t *testing.T) {
	m := &HierarchyMaster{
		rows: []HierarchyRow{
			{WorkType: "舗装工", Remarks: "舗設状況", SearchPatterns: "フィニッシャー"},
			{WorkType: "排水構造物工", Remarks: "設置状況"},
			{WorkType: "", Remarks: "安全朝礼"},
		},
	}

	// Case 1: Filter by WorkType -> should get 舗装工 row + empty WorkType row
	cands := m.GetCandidateRows([]string{"舗装工"}, nil)
	if len(cands) != 2 {
		t.Errorf("expected 2 candidates for WorkType, got %d", len(cands))
	}

	// Case 2: Filter by Keyword -> should get 舗装工 row (by keyword) + empty WorkType row (always included)
	cands2 := m.GetCandidateRows(nil, []string{"フィニッシャー"})
	if len(cands2) != 2 {
		t.Errorf("expected 2 candidates for keyword, got %d", len(cands2))
	}
}

func TestCalculateSimilarity(t *testing.T) {
	tests := []struct {
		a, b string
		min  float64
	}{
		{"朝礼実施状況", "安全朝礼実施状況", 0.5},
		{"舗装版破砕", "舗装版切断", 0.2},
		{"R0010850.JPG", "R0010850.JPG", 1.0},
	}

	for _, tt := range tests {
		got := CalculateSimilarity(tt.a, tt.b)
		if got < tt.min {
			t.Errorf("CalculateSimilarity(%q, %q) = %f; want >= %f", tt.a, tt.b, got, tt.min)
		}
	}
}

func TestNormalizeWorkType(t *testing.T) {
	if got := normalizeWorkType("舗装補修工"); got != "舗装工" {
		t.Errorf("normalizeWorkType(舗装補修工) = %q; want 舗装工", got)
	}
}

func TestExtractVarietyFromFolder(t *testing.T) {
	m := &HierarchyMaster{
		rows: []HierarchyRow{
			{Variety: "アスファルト舗装補修工"},
		},
	}
	got := extractVarietyFromFolder(m, "1.施工状況_舗装補修")
	if got != "アスファルト舗装補修工" {
		t.Errorf("extractVarietyFromFolder = %q; want アスファルト舗装補修工", got)
	}
}
