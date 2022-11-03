package qiniu

import "net/http"

type UserAgentTransport struct {
	transport http.RoundTripper
	userAgent string
}

func NewUserAgentTransport(userAgent string, transport http.RoundTripper) http.RoundTripper {
	return &UserAgentTransport{
		transport: transport,
		userAgent: userAgent,
	}
}

func (t *UserAgentTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Header.Get("User-Agent") == "" {
		request.Header.Set("User-Agent", t.userAgent)
	}
	innerTransport := t.transport
	if innerTransport == nil {
		innerTransport = http.DefaultTransport
	}
	return innerTransport.RoundTrip(request)
}
