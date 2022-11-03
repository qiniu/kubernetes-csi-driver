package qiniu

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/textproto"
	"sort"
	"strings"
	"time"
)

type credential struct {
	accessKey, secretKey string
}

type QiniuAuthTransport struct {
	transport   http.RoundTripper
	credential  credential
	useQBoxAuth bool
}

func NewQiniuAuthTransport(accessKey, secretKey string, transport http.RoundTripper, useQBoxAuth bool) http.RoundTripper {
	return &QiniuAuthTransport{
		transport:   transport,
		credential:  credential{accessKey: accessKey, secretKey: secretKey},
		useQBoxAuth: useQBoxAuth,
	}
}

func (t *QiniuAuthTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Header.Get("Authorization") == "" {
		var (
			auth string
			err  error
		)
		if t.useQBoxAuth {
			if auth, err = t.signQBoxRequest(request); err != nil {
				return nil, fmt.Errorf("QiniuAuthTransport.RoundTrip: sign request error via QBox: %w", err)
			}
		} else {
			if auth, err = t.signQiniuRequest(request); err != nil {
				return nil, fmt.Errorf("QiniuAuthTransport.RoundTrip: sign request error via Qiniu: %w", err)
			}
		}
		request.Header.Set("Authorization", auth)
	}
	innerTransport := t.transport
	if innerTransport == nil {
		innerTransport = http.DefaultTransport
	}
	return innerTransport.RoundTrip(request)
}

func (t *QiniuAuthTransport) signQBoxRequest(request *http.Request) (string, error) {
	return t.sign("QBox", func(writer io.Writer) (err error) {
		if _, err = writer.Write([]byte(request.URL.Path)); err != nil {
			return err
		}
		if request.URL.RawQuery != "" {
			if _, err = writer.Write([]byte("?")); err != nil {
				return err
			}
			if _, err = writer.Write([]byte(request.URL.RawQuery)); err != nil {
				return err
			}
		}
		if _, err = writer.Write([]byte("\n")); err != nil {
			return err
		}
		contentType := request.Header.Get("Content-Type")
		incBody := request.Body != nil && contentType == "application/x-www-form-urlencoded"
		if incBody {
			if body, err := t.readAllRequestBody(request); err != nil {
				return fmt.Errorf("QiniuAuthTransport.signQBoxRequest: read request body error: %w", err)
			} else {
				if _, err = writer.Write(body); err != nil {
					return err
				}
				request.Body = ioutil.NopCloser(bytes.NewReader(body))
			}
		}
		return nil
	})
}

func (t *QiniuAuthTransport) signQiniuRequest(request *http.Request) (string, error) {
	timeString := time.Now().UTC().Format("20060102T150405Z")
	request.Header.Set("X-Qiniu-Date", timeString)

	return t.sign("Qiniu", func(writer io.Writer) (err error) {
		if _, err = writer.Write([]byte(request.Method)); err != nil {
			return err
		}
		if _, err = writer.Write([]byte(" ")); err != nil {
			return err
		}
		if _, err = writer.Write([]byte(request.URL.Path)); err != nil {
			return err
		}
		if request.URL.RawQuery != "" {
			if _, err = writer.Write([]byte("?")); err != nil {
				return err
			}
			if _, err = writer.Write([]byte(request.URL.RawQuery)); err != nil {
				return err
			}
		}
		if _, err = writer.Write([]byte("\nHost: ")); err != nil {
			return err
		}
		if _, err = writer.Write([]byte(request.Host)); err != nil {
			return err
		}
		if _, err = writer.Write([]byte("\n")); err != nil {
			return err
		}
		contentType := request.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/x-www-form-urlencoded"
			request.Header.Set("Content-Type", contentType)
		}
		if _, err = writer.Write([]byte("Content-Type: ")); err != nil {
			return err
		}
		if _, err = writer.Write([]byte(contentType)); err != nil {
			return err
		}
		if _, err = writer.Write([]byte("\n")); err != nil {
			return err
		}

		xQiniuHeaders := make(xQiniuHeaders, 0, len(request.Header))
		for headerName := range request.Header {
			if len(headerName) > len("X-Qiniu-") && strings.HasPrefix(headerName, "X-Qiniu-") {
				xQiniuHeaders = append(xQiniuHeaders, xQiniuHeaderItem{
					HeaderName:  textproto.CanonicalMIMEHeaderKey(headerName),
					HeaderValue: request.Header.Get(headerName),
				})
			}
		}

		if len(xQiniuHeaders) > 0 {
			sort.Sort(xQiniuHeaders)
			for _, xQiniuHeader := range xQiniuHeaders {
				if _, err = writer.Write([]byte(xQiniuHeader.HeaderName)); err != nil {
					return err
				}
				if _, err = writer.Write([]byte(": ")); err != nil {
					return err
				}
				if _, err = writer.Write([]byte(xQiniuHeader.HeaderValue)); err != nil {
					return err
				}
				if _, err = writer.Write([]byte("\n")); err != nil {
					return err
				}
			}
		}
		if _, err = writer.Write([]byte("\n")); err != nil {
			return err
		}

		incBody := request.Body != nil && (contentType == "application/x-www-form-urlencoded" || contentType == "application/json")
		if incBody {
			if body, err := t.readAllRequestBody(request); err != nil {
				return fmt.Errorf("QiniuAuthTransport.signQiniuRequest: read request body error: %w", err)
			} else {
				if _, err = writer.Write(body); err != nil {
					return err
				}
				request.Body = ioutil.NopCloser(bytes.NewReader(body))
			}
		}

		return nil
	})
}

func (*QiniuAuthTransport) readAllRequestBody(request *http.Request) ([]byte, error) {
	if request.ContentLength == 0 {
		return nil, nil
	}
	if request.ContentLength > 0 {
		b := make([]byte, int(request.ContentLength))
		_, err := io.ReadFull(request.Body, b)
		return b, err
	}
	return ioutil.ReadAll(request.Body)
}

func (t *QiniuAuthTransport) sign(authName string, f func(io.Writer) error) (string, error) {
	h := hmac.New(sha1.New, []byte(t.credential.secretKey))
	buf := new(bytes.Buffer)
	w := io.MultiWriter(h, buf)
	if err := f(w); err != nil {
		return "", err
	}
	sign := base64.URLEncoding.EncodeToString(h.Sum(nil))
	return fmt.Sprintf("%s %s:%s", authName, t.credential.accessKey, sign), nil
}

type (
	xQiniuHeaderItem struct {
		HeaderName  string
		HeaderValue string
	}
	xQiniuHeaders []xQiniuHeaderItem
)

func (headers xQiniuHeaders) Len() int {
	return len(headers)
}

func (headers xQiniuHeaders) Less(i, j int) bool {
	if headers[i].HeaderName < headers[j].HeaderName {
		return true
	} else if headers[i].HeaderName > headers[j].HeaderName {
		return false
	} else {
		return headers[i].HeaderValue < headers[j].HeaderValue
	}
}

func (headers xQiniuHeaders) Swap(i, j int) {
	headers[i], headers[j] = headers[j], headers[i]
}
