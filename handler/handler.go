package handler

import (
	"bufio"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/dinalt/clip"
)

type Params struct {
	ErrC  chan error
	PoolC chan struct{}
}

const (
	sBadResponse = 701 + iota
	sNoResult
	sBadURLScheme
	sBadURL
	sInvalidSize
)

func New(p Params) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var (
			err    error
			status int
		)
		defer func() {
			if rec := recover(); rec != nil {
				err = fmt.Errorf("recovered from: %+v", rec)
				status = http.StatusInternalServerError
			}
			finalize(w, status, p.ErrC, err)
		}()

		if r.Method != http.MethodGet {
			err = fmt.Errorf("bad request method: %s", r.Method)
			status = http.StatusMethodNotAllowed
			return
		}
		err = r.ParseForm()
		if err != nil {
			err = fmt.Errorf("r.ParseForm: %w", err)
			status = http.StatusBadRequest
			return
		}

		ctx := r.Context()
		select {
		case <-ctx.Done():
			if p.ErrC != nil {
				p.ErrC <- ctx.Err()
			}
			return
		case <-p.PoolC:
		}
		defer func() { p.PoolC <- struct{}{} }()

		url := r.Form.Get("url")
		query := r.Form.Get("query")
		withContainers := strings.ToLower(r.Form.Get("with_containers")) == "true"
		size := r.Form.Get("size")

		w.Header().Add("Content-Type", "application/pdf")

		bw := bufio.NewWriter(w)
		defer func() {
			err := bw.Flush()
			if err != nil {
				p.ErrC <- fmt.Errorf("bufio.Writer.Flush: %w", err)
			}
		}()

		err = clip.Convert(ctx, url, bw, clip.Params{
			Query:              query,
			PreserveContainers: withContainers,
			Size:               size,
		})
		if err != nil {
			var ignored *clip.IgnoredError
			if !errors.As(err, &ignored) {
				err = fmt.Errorf("clip.Convert: %w", err)
				status = http.StatusInternalServerError
				return
			}
		}

		status = http.StatusOK
	}
}

func finalize(w http.ResponseWriter, status int, errC chan<- error, err error) {
	if err != nil {
		errC <- err
	}
	if status == http.StatusOK {
		return
	}
	var (
		msg    string
		urlErr *clip.URLError
	)
	switch {
	case errors.Is(err, clip.ErrBadStatus):
		msg = "server returned non 2xx status for requested url"
		status = sBadResponse
	case errors.Is(err, clip.ErrBadURLScheme):
		msg = "bad URL scheme: only http and https are supported"
		status = sBadURLScheme
	case errors.Is(err, clip.ErrNoQueryResult):
		msg = "no result elements for given selectors"
		status = sNoResult
	case errors.Is(err, clip.ErrInvalidPageSize):
		msg = "invalid page size"
		status = sInvalidSize
	case errors.As(err, &urlErr):
		msg = "malformed url"
		status = sBadURL
	default:
		msg = http.StatusText(status)
	}
	w.WriteHeader(status)
	if err != nil {
		_, err := w.Write([]byte(msg))
		if err != nil {
			errC <- fmt.Errorf("write response: %w", err)
		}
	}
}
