package main

import (
	"context"
	"reflect"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

func TestHandleRequest(t *testing.T) {
	type args struct {
		ctx context.Context
		req events.APIGatewayProxyRequest
	}
	tests := []struct {
		name    string
		args    args
		wantRes events.APIGatewayProxyResponse
		wantErr bool
	}{
		{"no error", args{context.Background(), events.APIGatewayProxyRequest{
			HTTPMethod: "POST",
			Headers: map[string]string{
				"content-type": "application/json",
				"accept":       "application/octet-stream",
			},
			Body: "{\"url\":\"https://pkg.go.dev/log\"}",
		}},
			events.APIGatewayProxyResponse{
				StatusCode:        200,
				MultiValueHeaders: map[string][]string{"Content-Type": {"application/octet-stream"}},
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes, err := HandleRequest(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("HandleRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotRes.StatusCode != tt.wantRes.StatusCode {
				t.Errorf("HandleRequest().StatusCode = %v, want %v", gotRes.StatusCode,
					tt.wantRes.StatusCode)
			}
			if !tt.wantErr && len(gotRes.Body) == 0 {
				t.Errorf("HandleRequest() body is empty")
			}
			if !reflect.DeepEqual(gotRes.Headers, tt.wantRes.Headers) {
				t.Errorf("HandleRequest().Headers = %+v, want %+v", gotRes.Headers, tt.wantRes.Headers)
			}
			if !reflect.DeepEqual(gotRes.MultiValueHeaders, tt.wantRes.MultiValueHeaders) {
				t.Errorf("HandleRequest().MultiValueHeaders = %+v, want %+v", gotRes.MultiValueHeaders,
					tt.wantRes.MultiValueHeaders)
			}
			if t.Failed() && len(gotRes.Body) < 100 {
				t.Logf("HandleRequest().Body = %s", gotRes.Body)
			}
		})
	}
}
