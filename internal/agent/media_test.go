package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseSegmentation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		text    string
		want    []Box
		wantErr bool
	}{
		{"empty means no matches", "", nil, false},
		{
			"two objects with masks (docs format)",
			`<0f>0<|box;x1=353;y1=382;x2=453;y2=427;w=537;h=561|><|mask;x=0;y=0;data=46,101,~ab<>()|>,7<|box;x1=260;y1=10;x2=300;y2=90;w=537;h=561;c=0.91|><|mask;x=0;y=0;data=4,4,!xy|>`,
			[]Box{{ObjectID: 0, X1: 353, Y1: 382, X2: 453, Y2: 427}, {ObjectID: 7, X1: 260, Y1: 10, X2: 300, Y2: 90, Confidence: 0.91}},
			false,
		},
		{"missing coordinate fails", `<0f>0<|box;x1=1;y1=2;x2=3;w=9;h=9|>`, nil, true},
		{"empty box fails", `<0f>0<|box;x1=5;y1=5;x2=5;y2=9;w=9;h=9|>`, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseSegmentation(tc.text)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("box %d = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestGenerateImageAndSegment(t *testing.T) {
	png := []byte("\x89PNG fake")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]any
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			t.Errorf("decode: %v", err)
		}
		w.Header().Set("content-type", "application/json")
		switch r.URL.Path {
		case "/images/generations":
			if m["model"] != ImageModelID || m["output_format"] != "png" || m["response_format"] != "b64_json" {
				t.Errorf("image request = %v", m)
			}
			_, _ = w.Write([]byte(`{"created":1,"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(png) + `"}],"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}`))
		case "/responses":
			raw, _ := json.Marshal(m)
			if m["model"] != SegmentModelID || !strings.Contains(string(raw), `"sticky note"`) || !strings.Contains(string(raw), "data:image/png;base64,") {
				t.Errorf("segment request = %s", raw)
			}
			_, _ = w.Write([]byte(`{"id":"r","output":[{"type":"message","content":[{"type":"output_text","text":"<0f>3<|box;x1=1;y1=2;x2=30;y2=40;w=64;h=64|>"}]}],"usage":{"total_tokens":1}}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	cfg := Config{APIKey: "k", ModelID: "muse-spark-1.3", BaseURL: srv.URL}
	img, usage, err := GenerateImage(context.Background(), cfg, "a whiteboard", "")
	if err != nil || string(img) != string(png) || usage.TotalTokens != 3 {
		t.Fatalf("GenerateImage = %q, %+v, %v", img, usage, err)
	}
	boxes, _, err := Segment(context.Background(), cfg, ImageInput{Data: img, MIME: "image/png"}, "sticky note")
	if err != nil || len(boxes) != 1 || boxes[0].ObjectID != 3 || boxes[0].X2 != 30 {
		t.Fatalf("Segment = %+v, %v", boxes, err)
	}
}

func TestExtractFromImagesRetriesThenFailsClosed(t *testing.T) {
	spec := testSpec()
	for _, tc := range []struct {
		name    string
		replies []string
		wantErr bool
		wantN   int
	}{
		{"valid after one schema retry", []string{`{"status":"maybe"}`, `{"status":"ok"}`}, false, 2},
		{"fails closed after retries", []string{`nope`, `{"status":"x"}`, `{}`}, true, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var m map[string]any
				if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
					t.Errorf("decode: %v", err)
				}
				raw, _ := json.Marshal(m)
				if n == 0 && !strings.Contains(string(raw), `"input_image"`) {
					t.Errorf("first request carries no image: %s", raw)
				}
				if n > 0 && m["previous_response_id"] != "r" {
					t.Errorf("retry does not chain previous_response_id: %v", m["previous_response_id"])
				}
				reply, _ := json.Marshal(tc.replies[n])
				n++
				w.Header().Set("content-type", "application/json")
				_, _ = w.Write([]byte(`{"id":"r","output":[{"type":"message","content":[{"type":"output_text","text":` + string(reply) + `}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`))
			}))
			defer srv.Close()
			cfg := Config{APIKey: "k", ModelID: "muse-spark-1.3", BaseURL: srv.URL}
			res, err := ExtractFromImages(context.Background(), cfg, spec, "intake_summary", "read it",
				[]ImageInput{{Data: []byte("img"), MIME: "image/png"}})
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if n != tc.wantN {
				t.Errorf("requests = %d, want %d", n, tc.wantN)
			}
			if !tc.wantErr && (string(res.JSON) != `{"status":"ok"}` || res.Usage.Calls != 2) {
				t.Errorf("result = %s usage %+v", res.JSON, res.Usage)
			}
		})
	}
}
