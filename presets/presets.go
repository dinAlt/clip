package presets

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"

	"github.com/dinalt/clip"
)

type preset struct {
	URLRegexp string `json:"url_regexp,omitempty"`
	*regexp.Regexp
	*clip.Params
}

type Presets map[string]preset

func FromJSONFile(file string) (Presets, error) {
	f, err := os.Open(file) // nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("os.Open: %w", err)
	}
	defer func() { _ = f.Close() }()
	return FromJSON(f)
}

func FromJSON(r io.Reader) (Presets, error) {
	var res map[string]preset

	err := json.NewDecoder(r).Decode(&res)
	if err != nil {
		return nil, fmt.Errorf("json.Decoder.Decode: %w", err)
	}
	return res, nil
}

func (p Presets) ByName(n string) *clip.Params {
	return p[n].Params
}

func (p Presets) ForSite(url string) *clip.Params {
	for _, v := range p {
		if v.Regexp == nil {
			if v.URLRegexp == "" {
				continue
			}
			v.Regexp = regexp.MustCompile(v.URLRegexp)
		}
		if v.Regexp.MatchString(url) {
			return v.Params
		}
	}
	return nil
}
