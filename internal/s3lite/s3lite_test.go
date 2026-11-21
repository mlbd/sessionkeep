package s3lite

import (
	"context"
	"errors"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// The worked examples of "Signature Calculations for the Authorization Header" in the Amazon S3
// API reference: bucket examplebucket, the example key pair, 24 May 2013, region us-east-1.
func TestSignaturesMatchAmazonsPublishedExamples(t *testing.T) {
	const (
		access = "AKIAIOSFODNN7EXAMPLE"
		secret = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
		host   = "examplebucket.s3.amazonaws.com"
		empty  = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	)
	at := time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name, method, path string
		query              url.Values
		headers            map[string]string
		payload, want      string
	}{
		{"get object", "GET", "/test.txt", nil,
			map[string]string{"host": host, "range": "bytes=0-9", "x-amz-content-sha256": empty, "x-amz-date": "20130524T000000Z"},
			empty, "f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"},
		{"put object", "PUT", "/test$file.text", nil,
			map[string]string{"date": "Fri, 24 May 2013 00:00:00 GMT", "host": host, "x-amz-content-sha256": "44ce7dd67c959e0d3524ffac1771dfbba87d2b6b4b4e99e42034a8b803f8b072",
				"x-amz-date": "20130524T000000Z", "x-amz-storage-class": "REDUCED_REDUNDANCY"},
			"44ce7dd67c959e0d3524ffac1771dfbba87d2b6b4b4e99e42034a8b803f8b072", "98ad721746da40c64f1a55b78f14c238d841ea1380cd77a1b5971af0ece108bd"},
		{"get bucket lifecycle", "GET", "/", url.Values{"lifecycle": {""}},
			map[string]string{"host": host, "x-amz-content-sha256": empty, "x-amz-date": "20130524T000000Z"},
			empty, "fea454ca298b7da1c68078a5d1bdbfbbe0d65c699e0f91ac7a200a0136783543"},
		{"list objects", "GET", "/", url.Values{"max-keys": {"2"}, "prefix": {"J"}},
			map[string]string{"host": host, "x-amz-content-sha256": empty, "x-amz-date": "20130524T000000Z"},
			empty, "34b48302e7b5fa45bde8084f4b7868a86f0a534bc59db6670ed5711ef69dc6f7"},
	}
	for _, c := range cases {
		got := authorization(c.method, c.path, c.query, c.headers, c.payload, access, secret, "us-east-1", at)
		if !strings.HasSuffix(got, "Signature="+c.want) || !strings.HasPrefix(got, "AWS4-HMAC-SHA256 Credential="+access+"/20130524/us-east-1/s3/aws4_request,SignedHeaders=") {
			t.Errorf("%s: %s\nwant signature %s", c.name, got, c.want)
		}
	}
}

func TestEndpointsAreChecked(t *testing.T) {
	for _, ok := range []string{"https://abc123.r2.cloudflarestorage.com", "https://abc123.eu.r2.cloudflarestorage.com/", "http://127.0.0.1:9000", "http://localhost:9000"} {
		if _, err := CheckEndpoint(ok); err != nil {
			t.Errorf("%s refused: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "abc123.r2.cloudflarestorage.com", "http://abc123.r2.cloudflarestorage.com", "https://user:pw@host", "https://host/bucket", "https://host?x=1", "ftp://host"} {
		if _, err := CheckEndpoint(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

// A stand-in bucket (mock, not R2): objects are stored, listed in pages and never replaced, and
// a request signed with another secret is turned away.
func TestPutGetAndListAgainstAStandInBucket(t *testing.T) {
	fake := NewFake("AK", "SECRET")
	fake.PageSize = 2
	srv := httptest.NewServer(fake)
	defer srv.Close()
	c := &Client{Endpoint: srv.URL, Bucket: "team", Region: "auto", AccessKey: "AK", SecretKey: "SECRET"}
	ctx := context.Background()
	for _, k := range []string{"p/a b/1.age", "p/a b/2.age", "p/a b/3.age", "q/other.age"} {
		if err := c.Put(ctx, k, []byte("body of "+k)); err != nil {
			t.Fatalf("put %s: %v", k, err)
		}
	}
	if err := c.Put(ctx, "p/a b/1.age", []byte("replaced")); !errors.Is(err, ErrExists) {
		t.Fatalf("an existing object was replaced or the error is wrong: %v", err)
	}
	got, err := c.Get(ctx, "p/a b/1.age", 1<<20)
	if err != nil || string(got) != "body of p/a b/1.age" {
		t.Fatalf("get: %q %v", got, err)
	}
	if _, err := c.Get(ctx, "p/a b/1.age", 4); err == nil {
		t.Error("an object over the size limit was read")
	}
	list, err := c.List(ctx, "p/a b/")
	if err != nil || len(list) != 3 || list[0].Key != "p/a b/1.age" || list[2].Key != "p/a b/3.age" {
		t.Fatalf("list: %+v %v", list, err)
	}
	var se *StatusError
	if _, err := c.Get(ctx, "p/missing", 10); !errors.As(err, &se) || se.Status != 404 {
		t.Errorf("a missing object: %v", err)
	}
	wrong := *c
	wrong.SecretKey = "OTHER"
	if _, err := wrong.List(ctx, "p/"); !errors.As(err, &se) || se.Status != 403 {
		t.Errorf("a wrong secret: %v", err)
	}
}
