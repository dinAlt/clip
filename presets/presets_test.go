package presets

import (
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/dinalt/clip"
)

func TestFromJSON(t *testing.T) {
	type args struct {
		r io.Reader
	}
	tests := []struct {
		name    string
		args    args
		want    Presets
		wantErr bool
	}{
		{
			"it works",
			args{
				strings.NewReader(`{
					"test": {
						"url_regexp": ".+",
						"query": "queryString",
						"page_width": 10,
						"enable_javascript": true
					}
				}`),
			},
			Presets{
				"test": {
					URLRegexp: ".+",
					Params:    newParams("queryString", 10, true),
				},
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FromJSON(tt.args.r)
			if (err != nil) != tt.wantErr {
				t.Errorf("FromJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FromJSON() = %v, want %v", got, tt.want)
			}
		})
	}
}

func newParams(query string, pageWidth uint, enableJavascript bool) *clip.Params {
	return &clip.Params{
		Query:            &query,
		PageWidth:        &pageWidth,
		EnableJavascript: &enableJavascript,
	}
}
