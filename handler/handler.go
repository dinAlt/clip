package handler

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/dinalt/clip"
)

type Presets interface {
	ByName(string) *clip.Params
	ForSite(string) *clip.Params
}

type Params struct {
	PoolC  chan struct{}
	Logger Logger
	Presets
}

const contentType = "application/pdf"
const fallbackContentType = "application/octet-stream"

func (p *Params) validate() {
	if p.PoolC == nil {
		panic("clip/handler.Params: PoolC is nil")
	}
}

type Logger interface {
	Printf(format string, v ...interface{})
	Error(err error)
}

type dummyLogger struct{}

func (dummyLogger) Printf(string, ...interface{}) {}
func (dummyLogger) Error(error)                   {}

type dummyPresets struct{}

func (dummyPresets) ByName(string) *clip.Params  { return nil }
func (dummyPresets) ForSite(string) *clip.Params { return nil }

var (
	ErrBodyIsEmpty      = errors.New("request body is empty")
	ErrJSONUnmarshal    = errors.New("json unmarshal failed")
	ErrMethodNotAllowed = errors.New("method not allowed")
	ErrNoPreset         = errors.New("preset not found")
)

type ValueError struct {
	Inner    error
	Param    string
	Required string
}

func (e *ValueError) Error() string {
	if e.Inner == nil {
		return fmt.Sprintf("param: %s, required type: %s", e.Param, e.Required)
	}
	return fmt.Sprintf("%s, param: %s, required type: %s", e.Inner.Error(),
		e.Param, e.Required)
}

func (e *ValueError) Unwrap() error {
	return e.Inner
}

func New(p Params) http.HandlerFunc {
	p.validate()
	log := p.Logger
	if log == nil {
		log = dummyLogger{}
	}
	presets := p.Presets
	if presets == nil {
		presets = dummyPresets{}
	}
	poolC := p.PoolC

	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("new request: %v", r.URL)
		var err error
		defer func() {
			if rec := recover(); rec != nil {
				err = fmt.Errorf("recovered from: %+v", rec)
			}
			if r.Body != nil {
				_ = r.Body.Close()
			}
			finalize(w, log, err)
		}()

		var pReq *parsedRequest

		pReq, err = parse(r)
		if err != nil {
			err = fmt.Errorf("parse: %w", err)
			return
		}

		ctx := r.Context()
		select {
		case <-ctx.Done():
			log.Error(ctx.Err())
			return
		case <-poolC:
		}
		defer func() { poolC <- struct{}{} }()

		var ct = fallbackContentType
		if strings.Contains(strings.ToLower(r.Header.Get("accept")),
			contentType) {
			ct = contentType
		}
		w.Header().Add("content-type", ct)

		bw := bufio.NewWriter(w)
		defer func() {
			err := bw.Flush()
			if err != nil {
				log.Error(fmt.Errorf("bufio.Writer.Flush: %w", err))
			}
		}()

		var preset *clip.Params
		switch {
		case pReq.Preset == "":
		case pReq.Preset == "auto":
			preset = presets.ForSite(pReq.URL)
		default:
			preset = presets.ByName(pReq.Preset)
			if preset == nil {
				err = fmt.Errorf("%w: %s", ErrNoPreset, pReq.Preset)
				return
			}
		}
		if preset != nil {
			pReq.AddFrom(preset)
		}

		log.Printf("request: url: %s, preset: %s, params: %v",
			pReq.URL, pReq.Preset, pReq.Params)

		err = clip.ToPDFCtx(ctx, pReq.URL, bw, pReq.Params)
		if err != nil {
			var ignored *clip.IgnoredError
			if !errors.As(err, &ignored) {
				err = fmt.Errorf("clip.ToPDFCtx(ctx, %s, %v): %w", pReq.URL, pReq.Params, err)
				return
			}
		}
	}
}

const (
	SBadResponse = 701 + iota
	SNoResult
	SBadURLScheme
	SBadURL
	SValidationFailed
	SNoPreset
)

func finalize(w http.ResponseWriter, log Logger, err error) {
	if err != nil {
		log.Error(err)
	}
	var ignoredError *clip.IgnoredError
	if err == nil || errors.As(err, &ignoredError) {
		return
	}

	var (
		msg      string
		urlErr   *clip.URLError
		validErr *clip.ValidationError
		valErr   *ValueError
		status   int
	)
	switch {
	case errors.Is(err, clip.ErrBadStatus):
		msg = "server returned non 2xx status for requested url"
		status = SBadResponse
	case errors.Is(err, clip.ErrNoURL):
		msg = "url is required"
		status = http.StatusBadRequest
	case errors.Is(err, clip.ErrBadURLScheme):
		msg = "bad URL scheme: only http and https are supported"
		status = SBadURLScheme
	case errors.Is(err, clip.ErrNoQueryResult):
		msg = "no result elements for given selectors"
		status = SNoResult
	case errors.Is(err, ErrBodyIsEmpty):
		msg = "request body is empty"
		status = http.StatusBadRequest
	case errors.Is(err, ErrJSONUnmarshal):
		msg = "bad json value"
		status = http.StatusBadRequest
	case errors.Is(err, ErrMethodNotAllowed):
		status = http.StatusMethodNotAllowed
	case errors.Is(err, ErrNoPreset):
		status = SNoPreset
		msg = "preset not found"
	case errors.As(err, &validErr):
		msg = validErr.Message
		status = SValidationFailed
	case errors.As(err, &urlErr):
		msg = "malformed url"
		status = SBadURL
	case errors.As(err, &valErr):
		msg = fmt.Sprintf("validation error: param %s requires value of %s",
			valErr.Param, valErr.Required)
		status = SValidationFailed
	default:
		status = http.StatusInternalServerError
	}
	if msg == "" {
		msg = http.StatusText(status)
	}
	w.Header().Set("content-type", "text/plain")
	w.WriteHeader(status)
	_, err = w.Write([]byte(msg))
	if err != nil {
		log.Error(fmt.Errorf("write response: %w", err))
	}
}

type parsedRequest struct {
	URL    string `json:"url,omitempty"`
	Preset string `json:"preset,omitempty"`
	*clip.Params
}

func parse(r *http.Request) (*parsedRequest, error) {
	switch {
	case r.Method == "POST" && r.Header.Get("content-type") == "application/json":
		return parseJSON(r)
	case r.Method == "GET" || r.Method == "POST":
		fmt.Println("parsing form")
		return parseForm(r)
	}
	return nil, ErrMethodNotAllowed
}

func parseJSON(r *http.Request) (*parsedRequest, error) {
	res := parsedRequest{Params: &clip.Params{}}
	if r.Body == nil {
		return nil, ErrBodyIsEmpty
	}
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("ioutil.ReadAll: %w", err)
	}
	err = json.Unmarshal(b, &res)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrJSONUnmarshal, err)
	}
	return &res, nil
}

func parseForm(r *http.Request) (*parsedRequest, error) {
	err := r.ParseForm()
	if err != nil {
		return nil, fmt.Errorf("r.ParseForm: %w", err)
	}
	res := &clip.Params{}
	pt := reflect.TypeOf(res).Elem()
	pv := reflect.ValueOf(res)
	fcnt := pt.NumField()
	for i := 0; i < fcnt; i++ {
		jsonTag := pt.Field(i).Tag.Get("json")
		if jsonTag == "" {
			continue
		}
		fieldName := strings.Split(jsonTag, ",")[0]
		if fieldName == "-" {
			continue
		}
		reqv := r.Form.Get(fieldName)
		if reqv == "" {
			continue
		}
		fldt := pt.Field(i)
		kind := fldt.Type.Kind()
		if kind == reflect.Ptr {
			kind = fldt.Type.Elem().Kind()
		}

		var newV reflect.Value
		switch kind {
		case reflect.Uint:
			v, err := strconv.ParseUint(reqv, 10, 32)
			if err != nil {
				return nil, &ValueError{err, fieldName, "unsigned integer"}
			}
			uiv := uint(v)
			newV = reflect.ValueOf(&uiv)
		case reflect.String:
			newV = reflect.ValueOf(&reqv)
		case reflect.Bool:
			var boolv bool
			switch strings.ToLower(reqv) {
			case "true":
				boolv = true
			case "false":
			default:
				return nil, &ValueError{nil, fieldName, "bool (true or false)"}
			}
			newV = reflect.ValueOf(&boolv)
		}
		pv.Elem().Field(i).Set(newV)
	}

	return &parsedRequest{
		Preset: r.Form.Get("preset"),
		URL:    r.Form.Get("url"),
		Params: res,
	}, nil
}
