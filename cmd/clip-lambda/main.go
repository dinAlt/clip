package main

import (
	"bytes"
	"context"
	"os"

	"github.com/aws/aws-lambda-go/lambda"

	"github.com/dinalt/clip"
)

type Event struct {
	URL string
	clip.Params
}

func HandleRequest(ctx context.Context, event Event) ([]byte, error) {
	err := os.Setenv("WKHTMLTOPDF_PATH", os.Getenv("LAMBDA_TASK_ROOT"))
	if err != nil {
		return nil, err
	}
	b := bytes.Buffer{}
	err = clip.ConvertCtx(ctx, event.URL, &b, &event.Params)
	if err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func main() {
	lambda.Start(HandleRequest)
}
