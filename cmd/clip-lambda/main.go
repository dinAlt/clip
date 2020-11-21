package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/dinalt/clip/handler"
	"github.com/dinalt/clip/presets"
)

func HandleRequest(ctx context.Context, req events.APIGatewayProxyRequest) (res events.APIGatewayProxyResponse, err error) {
	var (
		body    string
		headers map[string][]string
		status  = http.StatusInternalServerError
	)
	defer func() {
		res = events.APIGatewayProxyResponse{
			StatusCode:        mapCustomStatus(status),
			Body:              body,
			MultiValueHeaders: headers,
			IsBase64Encoded:   status == http.StatusOK,
		}
	}()
	lroot := os.Getenv("LAMBDA_TASK_ROOT")
	err = os.Setenv("WKHTMLTOPDF_PATH", lroot)
	if err != nil {
		err = fmt.Errorf("os.Setenv: %w", err)
		return
	}
	ps, err := presets.FromJSONFile(filepath.Join(lroot, "presets.json"))
	if err != nil {
		errLogger.Printf("presets.FromJSONFile: %s", err.Error())
		if !errors.Is(err, os.ErrNotExist) {
			return
		}
	}
	r, err := makeReq(ctx, &req)
	if err != nil {
		err = fmt.Errorf("makeReq: %w", err)
	}
	poolC := make(chan struct{}, 1)
	poolC <- struct{}{}
	h := handler.New(handler.Params{
		PoolC:   poolC,
		Logger:  logger{},
		Presets: ps,
	})

	w := responseWriter{}
	h.ServeHTTP(&w, r)

	status = w.code
	headers = w.header

	if status != 0 {
		body = w.String()
		return
	}

	status = http.StatusOK
	body = base64.StdEncoding.EncodeToString(w.Bytes())

	return
}

func mapCustomStatus(n int) int {
	if n < 600 {
		return n
	}
	if n == handler.SNoResult {
		return http.StatusNoContent
	}
	return http.StatusBadRequest
}
func main() {
	lambda.Start(HandleRequest)
}

var errLogger = log.New(os.Stderr, "ERROR ", log.Llongfile)

type logger struct{}

func (l logger) Printf(format string, v ...interface{}) {}

func (l logger) Error(err error) {
	errLogger.Println(err.Error())
}

type responseWriter struct {
	code int
	bytes.Buffer
	header http.Header
}

func (w *responseWriter) WriteHeader(n int) {
	w.code = n
}

func (w *responseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func makeReq(ctx context.Context, awsReq *events.APIGatewayProxyRequest) (*http.Request, error) {
	u := url.URL{}
	u.Path = awsReq.Path

	fmt.Println("len:", len(awsReq.QueryStringParameters))
	vs := make(url.Values)
	for k, v := range awsReq.QueryStringParameters {
		vs.Add(k, v)
	}
	u.RawQuery = vs.Encode()
	r, err := http.NewRequestWithContext(ctx, awsReq.HTTPMethod, u.String(), strings.NewReader(awsReq.Body))
	if err != nil {
		return nil, err
	}
	for k, v := range awsReq.Headers {
		r.Header.Add(k, v)
	}
	for k, v := range awsReq.MultiValueHeaders {
		for _, hv := range v {
			r.Header.Add(k, hv)
		}
	}
	return r, nil
}
