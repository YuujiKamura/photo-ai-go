package engine

import (
	"testing"
)

func TestParseEngineResponse(t *testing.T) {
	type dummyData struct {
		Path  string `json:"path"`
		Count int    `json:"count"`
	}

	tests := []struct {
		name    string
		output  string
		want    dummyData
		wantErr bool
	}{
		{
			name:   "single_line_json",
			output: `{"ok":true,"data":{"path":"out.pdf","count":3}}`,
			want:   dummyData{Path: "out.pdf", Count: 3},
		},
		{
			name:   "multi_line_with_logs",
			output: "Starting engine...\nWorking...\n{\"ok\":true,\"data\":{\"path\":\"multi.pdf\",\"count\":5}}\nDone.",
			want:   dummyData{Path: "multi.pdf", Count: 5},
		},
		{
			name:    "engine_error",
			output:  `{"ok":false,"error":"something went wrong"}`,
			wantErr: true,
		},
		{
			name:    "invalid_json",
			output:  `{"ok":true,"data":{invalid}`,
			wantErr: true,
		},
		{
			name:    "empty_output",
			output:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got dummyData
			err := parseEngineResponse([]byte(tt.output), &got)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseEngineResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestHelpMentionsToken(t *testing.T) {
	help := `Usage: photo-ai [COMMAND]
Commands:
  analyze  Analyze photos
  export   Export results
  pair     Pair photos`

	if !helpMentionsToken(help, "analyze") {
		t.Error("expected help to mention analyze")
	}
	if !helpMentionsToken(help, "export") {
		t.Error("expected help to mention export")
	}
	if helpMentionsToken(help, "unknown") {
		t.Error("did not expect help to mention unknown")
	}
}
