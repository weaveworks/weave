package proxy

import "net/http"

type interceptor interface {
	InterceptRequest(*http.Request) (*http.Request, error)
	InterceptResponse(*http.Response) (*http.Response, error)
}

type nullInterceptor struct {
}

func (i nullInterceptor) InterceptRequest(r *http.Request) (*http.Request, error) {
	return r, nil
}

func (i nullInterceptor) InterceptResponse(r *http.Response) (*http.Response, error) {
	return r, nil
}
