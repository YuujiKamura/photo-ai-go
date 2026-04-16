package tagger

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/YuujiKamura/photo-ai-go/internal/matching"
	"github.com/YuujiKamura/resident-agent/pkg/api"
)

// MapToMaster uses the AI to select the best master entry from a candidate group.
func MapToMaster(ctx context.Context, photoPath string, detectedText, description string, candidates []matching.HierarchyRow) (*matching.HierarchyRow, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	// 1. Build a high-quality candidate list with full context
	var sb strings.Builder
	for i, c := range candidates {
		// Format: [Index] Category | WorkType > Variety > Subphase > Remarks
		parts := []string{}
		if c.PhotoDivision != "" { parts = append(parts, "["+c.PhotoDivision+"]") }
		if c.PhotoType != "" { parts = append(parts, c.PhotoType) }
		
		hierarchy := []string{}
		if c.WorkType != "" { hierarchy = append(hierarchy, c.WorkType) }
		if c.Variety != "" { hierarchy = append(hierarchy, c.Variety) }
		if c.Subphase != "" { hierarchy = append(hierarchy, c.Subphase) }
		if c.Remarks != "" { hierarchy = append(hierarchy, c.Remarks) }
		
		fullLine := fmt.Sprintf("[%d] %s: %s (キーワード: %s)", 
			i, strings.Join(parts, " | "), strings.Join(hierarchy, " > "), c.SearchPatterns)
		fmt.Fprintln(&sb, fullLine)
	}

	prompt := fmt.Sprintf(`
あなたは土木工事写真の整理エキスパートです。
写真の内容（OCRとAI説明）を読み取り、提示された工種マスタの中から最も適切な「1行」を選択してください。

【判定のヒント】
- 黒板（看板）が写っている写真は、通常「施工状況写真」や「出来形管理写真」です。
- 作業員が集まっている、または安全看板のみの場合は「安全管理写真」です。
- 「舗装厚」「厚さ」「t=」という言葉があれば、舗装版破砕または表層工の「厚さ確認」です。
- 「剥取」「積込」があれば、舗装版破砕です。

【写真の内容】
- OCRテキスト: %s
- 写真の説明: %s

【マスタ候補リスト】
%s

【回答ルール】
- 該当する番号（[ ]内の数字）のみを回答してください。
- 該当なしの場合は "-1" を回答。
`, detectedText, description, sb.String())

	opts := api.DefaultOptions().WithModel(GetDefaultModel())
	res, err := api.Prompt(prompt, opts)
	if err != nil {
		return nil, err
	}

	// 2. Parse the selected index using regex
	res = strings.TrimSpace(res)
	re := regexp.MustCompile(`-?\d+`)
	match := re.FindString(res)
	if match == "" {
		return nil, fmt.Errorf("AI returned no numeric index: %q", res)
	}

	var idx int
	if _, err := fmt.Sscanf(match, "%d", &idx); err != nil {
		return nil, err
	}

	if idx >= 0 && idx < len(candidates) {
		return &candidates[idx], nil
	}

	return nil, nil
}
