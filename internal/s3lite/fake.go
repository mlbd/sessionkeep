package s3lite

import (
	"encoding/xml"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Fake is a stand-in bucket for tests: an HTTP handler that keeps objects in memory and checks
// each request's signature the way the real service does, by computing it again from what
// arrived. It is not R2 and proves nothing about R2 beyond the protocol as SessionKeep speaks it.
type Fake struct {
	AccessKey, SecretKey string
	PageSize             int

	mu      sync.Mutex
	Objects map[string][]byte // "bucket/key" -> body
}

// NewFake returns an empty stand-in bucket that accepts one key pair.
func NewFake(accessKey, secretKey string) *Fake {
	return &Fake{AccessKey: accessKey, SecretKey: secretKey, PageSize: 1000, Objects: map[string][]byte{}}
}

func (f *Fake) deny(w http.ResponseWriter, status int, code string) {
	w.WriteHeader(status)
	io.WriteString(w, "<Error><Code>"+code+"</Code><Message>stand-in bucket</Message></Error>")
}

func (f *Fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<30))
	auth := r.Header.Get("Authorization")
	_, rest, _ := strings.Cut(auth, "SignedHeaders=")
	signedList, _, _ := strings.Cut(rest, ",")
	headers := map[string]string{}
	for _, name := range strings.Split(signedList, ";") {
		if name == "host" {
			headers[name] = r.Host
		} else {
			headers[name] = r.Header.Get(name)
		}
	}
	at, err := time.Parse("20060102T150405Z", r.Header.Get("x-amz-date"))
	path, _ := url.PathUnescape(r.URL.EscapedPath())
	if err != nil || hashHex(body) != r.Header.Get("x-amz-content-sha256") ||
		authorization(r.Method, path, r.URL.Query(), headers, hashHex(body), f.AccessKey, f.SecretKey, "auto", at) != auth {
		f.deny(w, http.StatusForbidden, "SignatureDoesNotMatch")
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	name := strings.TrimPrefix(path, "/")
	bucket, key, hasKey := strings.Cut(name, "/")
	switch {
	case r.Method == http.MethodPut && hasKey:
		if _, exists := f.Objects[name]; exists && r.Header.Get("If-None-Match") == "*" {
			f.deny(w, http.StatusPreconditionFailed, "PreconditionFailed")
			return
		}
		f.Objects[name] = body
	case r.Method == http.MethodGet && hasKey:
		data, ok := f.Objects[name]
		if !ok {
			f.deny(w, http.StatusNotFound, "NoSuchKey")
			return
		}
		w.Write(data)
	case r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2":
		prefix, token := bucket+"/"+r.URL.Query().Get("prefix"), r.URL.Query().Get("continuation-token")
		var keys []string
		for k := range f.Objects {
			if strings.HasPrefix(k, prefix) {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		start, _ := strconv.Atoi(token)
		type entry struct {
			Key  string `xml:"Key"`
			Size int    `xml:"Size"`
		}
		var res struct {
			XMLName     xml.Name `xml:"ListBucketResult"`
			Contents    []entry  `xml:"Contents"`
			IsTruncated bool     `xml:"IsTruncated"`
			Next        string   `xml:"NextContinuationToken,omitempty"`
		}
		for i := start; i < len(keys) && i < start+f.PageSize; i++ {
			res.Contents = append(res.Contents, entry{Key: strings.TrimPrefix(keys[i], bucket+"/"), Size: len(f.Objects[keys[i]])})
		}
		if start+f.PageSize < len(keys) {
			res.IsTruncated, res.Next = true, strconv.Itoa(start+f.PageSize)
		}
		xml.NewEncoder(w).Encode(res)
	default:
		_ = key
		f.deny(w, http.StatusBadRequest, "NotImplemented")
	}
}
