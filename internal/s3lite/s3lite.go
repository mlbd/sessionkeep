// Package s3lite is the small part of the S3 protocol SessionKeep needs to keep files in a bucket
// the owner chose (Cloudflare R2 first): put a new object, get one, list a prefix. Requests are
// signed with AWS Signature Version 4, which the tests check against Amazon's published examples.
// Objects are addressed path-style (endpoint/bucket/key).
package s3lite

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Client talks to one bucket.
type Client struct {
	Endpoint  string // https://ACCOUNT.r2.cloudflarestorage.com
	Bucket    string
	Region    string // "auto" for R2
	AccessKey string
	SecretKey string
	HTTP      *http.Client
	Now       func() time.Time
}

// Object is one listed object.
type Object struct {
	Key  string
	Size int64
}

// ErrExists means Put found an object already at that key and left it alone.
var ErrExists = errors.New("an object with that name already exists")

// CheckEndpoint returns the endpoint in the form requests use, or says what is wrong with it.
// Only https is accepted, except for this computer itself (which is what the tests talk to).
func CheckEndpoint(raw string) (string, error) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(raw), "/"))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", fmt.Errorf("the endpoint must look like https://ACCOUNT.r2.cloudflarestorage.com, with nothing after the host")
	}
	local := u.Hostname() == "localhost"
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		local = ip.IsLoopback()
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return "", fmt.Errorf("the endpoint must start with https://")
	}
	return u.Scheme + "://" + u.Host, nil
}

// encode is the URI encoding Signature Version 4 prescribes: everything but A-Z a-z 0-9 - _ . ~
// becomes %XX in upper case; a slash is kept only inside a path.
func encode(s string, keepSlash bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '_', c == '.', c == '~', c == '/' && keepSlash:
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func hmacSHA(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func hashHex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// canonicalQuery sorts and encodes query parameters.
func canonicalQuery(q url.Values) string {
	var parts []string
	for k, vs := range q {
		for _, v := range vs {
			parts = append(parts, encode(k, false)+"="+encode(v, false))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, "&")
}

// authorization computes the Authorization header value for a request. headers must hold every
// header that is sent and signed, host included, with lower-case names.
func authorization(method, path string, q url.Values, headers map[string]string, payloadHash, accessKey, secretKey, region string, at time.Time) string {
	names := make([]string, 0, len(headers))
	for k := range headers {
		names = append(names, k)
	}
	sort.Strings(names)
	var canon strings.Builder
	for _, k := range names {
		canon.WriteString(k + ":" + strings.Join(strings.Fields(headers[k]), " ") + "\n")
	}
	signed := strings.Join(names, ";")
	request := strings.Join([]string{method, encode(path, true), canonicalQuery(q), canon.String(), signed, payloadHash}, "\n")
	day, stamp := at.UTC().Format("20060102"), at.UTC().Format("20060102T150405Z")
	scope := day + "/" + region + "/s3/aws4_request"
	toSign := "AWS4-HMAC-SHA256\n" + stamp + "\n" + scope + "\n" + hashHex([]byte(request))
	key := hmacSHA(hmacSHA(hmacSHA(hmacSHA([]byte("AWS4"+secretKey), day), region), "s3"), "aws4_request")
	return "AWS4-HMAC-SHA256 Credential=" + accessKey + "/" + scope + ",SignedHeaders=" + signed + ",Signature=" + hex.EncodeToString(hmacSHA(key, toSign))
}

// StatusError is an answer from the storage service that is not a success.
type StatusError struct {
	Status  int
	Code    string
	Message string
}

func (e *StatusError) Error() string {
	hint := ""
	switch e.Status {
	case 401, 403:
		hint = " (the access key, its secret, or its permission for this bucket is wrong)"
	case 404:
		hint = " (the bucket or the object does not exist)"
	}
	return fmt.Sprintf("the storage service answered %d %s %s%s", e.Status, e.Code, e.Message, hint)
}

func (c *Client) do(ctx context.Context, method, key string, q url.Values, body []byte, extra map[string]string) (*http.Response, error) {
	endpoint, err := CheckEndpoint(c.Endpoint)
	if err != nil {
		return nil, err
	}
	u, _ := url.Parse(endpoint)
	path := "/" + c.Bucket
	if key != "" {
		path += "/" + key
	}
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	at := now()
	payload := hashHex(body)
	headers := map[string]string{"host": u.Host, "x-amz-content-sha256": payload, "x-amz-date": at.UTC().Format("20060102T150405Z")}
	for k, v := range extra {
		headers[strings.ToLower(k)] = v
	}
	target := endpoint + encode(path, true)
	if len(q) > 0 {
		target += "?" + canonicalQuery(q)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		if k != "host" {
			req.Header.Set(k, v)
		}
	}
	req.Header.Set("Authorization", authorization(method, path, q, headers, payload, c.AccessKey, c.SecretKey, c.Region, at))
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("the storage service could not be reached: %w", err)
	}
	if resp.StatusCode/100 == 2 {
		return resp, nil
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	var e struct {
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
	}
	xml.Unmarshal(raw, &e)
	return nil, &StatusError{Status: resp.StatusCode, Code: e.Code, Message: e.Message}
}

// Put stores body at key, and only if nothing is there yet: an existing object is never replaced.
func (c *Client) Put(ctx context.Context, key string, body []byte) error {
	resp, err := c.do(ctx, http.MethodPut, key, nil, body, map[string]string{"if-none-match": "*"})
	var se *StatusError
	if errors.As(err, &se) && se.Status == http.StatusPreconditionFailed {
		return ErrExists
	}
	if err != nil {
		return err
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	return resp.Body.Close()
}

// Get returns the object at key, refusing one larger than max bytes.
func (c *Client) Get(ctx context.Context, key string, max int64) ([]byte, error) {
	resp, err := c.do(ctx, http.MethodGet, key, nil, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > max {
		return nil, fmt.Errorf("%s is larger than %d bytes and was not read", key, max)
	}
	return raw, nil
}

// List returns every object whose key starts with prefix, in key order.
func (c *Client) List(ctx context.Context, prefix string) ([]Object, error) {
	var out []Object
	token := ""
	for page := 0; page < 1000; page++ {
		q := url.Values{"list-type": {"2"}, "prefix": {prefix}}
		if token != "" {
			q.Set("continuation-token", token)
		}
		resp, err := c.do(ctx, http.MethodGet, "", q, nil, nil)
		if err != nil {
			return nil, err
		}
		raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		var res struct {
			Contents []struct {
				Key  string `xml:"Key"`
				Size int64  `xml:"Size"`
			} `xml:"Contents"`
			IsTruncated bool   `xml:"IsTruncated"`
			Next        string `xml:"NextContinuationToken"`
		}
		if err := xml.Unmarshal(raw, &res); err != nil {
			return nil, fmt.Errorf("the storage service's listing could not be read: %w", err)
		}
		for _, o := range res.Contents {
			if strings.HasPrefix(o.Key, prefix) {
				out = append(out, Object{Key: o.Key, Size: o.Size})
			}
		}
		if !res.IsTruncated || res.Next == "" {
			break
		}
		token = res.Next
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}
