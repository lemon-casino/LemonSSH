package cloudsync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testS3AccessKey = "AKIDEXAMPLE"
	testS3SecretKey = "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY"
	testS3Region    = "us-east-1"
	testS3Bucket    = "netcatty"
)

// TestSigningKeyAWSVector pins the AWS-documented Signature V4 key-derivation
// example: secret "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY", date 20150830,
// region us-east-1, service iam -> c4afb1cc...a4b9. The s3-service value for
// the same date/region is pinned alongside it. Both hex values were
// cross-checked against an independent reference implementation of the
// documented HMAC chain (the example is published for the iam service; no
// correct derivation maps 20150830/us-east-1/s3 onto that key).
func TestSigningKeyAWSVector(t *testing.T) {
	cases := []struct {
		service string
		want    string
	}{
		{"iam", "c4afb1cc5771d871763a393e44b703571b55cc28424d1a5e86da6ed3c154a4b9"},
		{"s3", "32f78051dcde24c552811d654f4a769112bb834b03975cdd6b1fd7d16248c269"},
	}
	for _, tc := range cases {
		key := signingKey(testS3SecretKey, "20150830", testS3Region, tc.service)
		if got := hex.EncodeToString(key); got != tc.want {
			t.Fatalf("signing key (service %s) = %s, want %s", tc.service, got, tc.want)
		}
	}
}

// TestCanonicalRequestKnownPut pins the exact canonical request shape for a
// known PUT: the empty query line, sorted canonical headers, the signed
// headers list and the payload hash -- plus the session-token variant.
func TestCanonicalRequestKnownPut(t *testing.T) {
	payloadHash := sha256.Sum256([]byte("v1"))
	payloadHashHex := hex.EncodeToString(payloadHash[:])
	headers := http.Header{}
	headers.Set("Host", "127.0.0.1:9000")
	headers.Set("X-Amz-Date", "20150830T123600Z")
	headers.Set("X-Amz-Content-Sha256", payloadHashHex)

	canonical, signedHeaders := canonicalRequest("PUT", "/netcatty/snapshots/snapshot.json", "", headers, payloadHashHex)
	want := strings.Join([]string{
		"PUT",
		"/netcatty/snapshots/snapshot.json",
		"",
		"host:127.0.0.1:9000",
		"x-amz-content-sha256:" + payloadHashHex,
		"x-amz-date:20150830T123600Z",
		"",
		"host;x-amz-content-sha256;x-amz-date",
		payloadHashHex,
	}, "\n")
	if canonical != want {
		t.Fatalf("canonical request:\n%s\nwant:\n%s", canonical, want)
	}
	if signedHeaders != "host;x-amz-content-sha256;x-amz-date" {
		t.Fatalf("signed headers = %q", signedHeaders)
	}

	// The session token is signed too and sorts last among x-amz-* names.
	headers.Set("X-Amz-Security-Token", "tok123")
	_, signedTokenHeaders := canonicalRequest("PUT", "/netcatty/snapshots/snapshot.json", "", headers, payloadHashHex)
	if signedTokenHeaders != "host;x-amz-content-sha256;x-amz-date;x-amz-security-token" {
		t.Fatalf("signed headers with token = %q", signedTokenHeaders)
	}
}

type s3blob struct {
	body []byte
	etag string
}

// fakeS3 is a minimal S3-compatible endpoint: one bucket in memory, strong
// ETags, If-Match enforcement, ListObjectsV2 XML and full SigV4 signature
// verification of every request (mismatches surface as 403).
type fakeS3 struct {
	mu           sync.Mutex
	objects      map[string]s3blob
	nextID       int
	hits         int
	lastAuth     string
	lastHost     string
	lastPath     string
	lastToken    string
	lastSigError string
}

func newFakeS3() *fakeS3 {
	return &fakeS3{objects: make(map[string]s3blob)}
}

const listV2XML = `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult>
  <Name>netcatty</Name>
  <Prefix>snapshots/</Prefix>
  <IsTruncated>false</IsTruncated>
  <Contents><Key>snapshots/old.json</Key></Contents>
  <Contents><Key>snapshots/snapshot.json</Key></Contents>
</ListBucketResult>`

func (f *fakeS3) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.hits++
		f.lastAuth = r.Header.Get("Authorization")
		f.lastHost = r.Host
		f.lastPath = r.URL.Path
		f.lastToken = r.Header.Get("X-Amz-Security-Token")
		if mismatch := checkSignature(r, testS3SecretKey, testS3Region); mismatch != "" {
			f.lastSigError = mismatch
			w.WriteHeader(http.StatusForbidden)
			return
		}
		f.lastSigError = ""

		switch r.Method {
		case http.MethodPut:
		case http.MethodGet, http.MethodHead, http.MethodDelete:
			if r.Header.Get("x-amz-content-sha256") != emptyPayloadSHA256 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}

		if r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2" {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(listV2XML))
			return
		}

		_, key, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
		switch r.Method {
		case http.MethodGet, http.MethodHead:
			blob, ok := f.objects[key]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("ETag", blob.etag)
			w.WriteHeader(http.StatusOK)
			if r.Method == http.MethodGet {
				_, _ = w.Write(blob.body)
			}
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			if r.Header.Get("x-amz-content-sha256") != sha256Hex(body) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if existing, ok := f.objects[key]; ok {
				if match := r.Header.Get("If-Match"); match != "" && match != existing.etag {
					w.WriteHeader(http.StatusPreconditionFailed)
					return
				}
			}
			f.nextID++
			etag := strconv.Quote("e" + strconv.Itoa(f.nextID))
			f.objects[key] = s3blob{body: body, etag: etag}
			w.Header().Set("ETag", etag)
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			delete(f.objects, key)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

// checkSignature recomputes the SigV4 signature from the incoming request and
// compares it with the Authorization header, returning "" when it verifies.
func checkSignature(r *http.Request, secretAccessKey, region string) string {
	const prefix = "AWS4-HMAC-SHA256 "
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, prefix) {
		return "missing AWS4-HMAC-SHA256 scheme"
	}
	var credential, signedHeaders, signature string
	for _, part := range strings.Split(strings.TrimPrefix(auth, prefix), ", ") {
		switch {
		case strings.HasPrefix(part, "Credential="):
			credential = strings.TrimPrefix(part, "Credential=")
		case strings.HasPrefix(part, "SignedHeaders="):
			signedHeaders = strings.TrimPrefix(part, "SignedHeaders=")
		case strings.HasPrefix(part, "Signature="):
			signature = strings.TrimPrefix(part, "Signature=")
		}
	}
	scopeParts := strings.Split(credential, "/")
	if len(scopeParts) != 5 || scopeParts[3] != "s3" || scopeParts[4] != "aws4_request" {
		return "unexpected credential scope " + credential
	}
	dateStamp, gotRegion := scopeParts[1], scopeParts[2]
	if gotRegion != region {
		return "unexpected region " + gotRegion
	}

	headers := http.Header{}
	headers.Set("Host", r.Host)
	for _, name := range strings.Split(signedHeaders, ";") {
		if name == "host" {
			continue
		}
		headers.Set(name, r.Header.Get(name))
	}
	canonical, recomputed := canonicalRequest(r.Method, r.URL.EscapedPath(), r.URL.RawQuery, headers, r.Header.Get("x-amz-content-sha256"))
	if recomputed != signedHeaders {
		return "signed headers mismatch: " + recomputed + " vs " + signedHeaders
	}
	scope := dateStamp + "/" + gotRegion + "/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + r.Header.Get("x-amz-date") + "\n" + scope + "\n" + sha256Hex([]byte(canonical))
	key := signingKey(secretAccessKey, dateStamp, gotRegion, "s3")
	want := hex.EncodeToString(hmacSHA256(key, []byte(stringToSign)))
	if want != signature {
		return "signature mismatch"
	}
	return ""
}

// TestS3RoundTripAndConflict walks the client contract against an in-memory
// S3: PUT/GET/HEAD/DELETE, If-Match conflict, 404 mapping, ListObjectsV2 and
// server-side SigV4 verification of every request.
func TestS3RoundTripAndConflict(t *testing.T) {
	fake := newFakeS3()
	server := httptest.NewServer(fake.handler())
	defer server.Close()

	client := NewS3Client(S3Config{
		Endpoint:        server.URL,
		Region:          testS3Region,
		Bucket:          testS3Bucket,
		AccessKeyID:     testS3AccessKey,
		SecretAccessKey: testS3SecretKey,
		UsePathStyle:    true,
	})
	ctx := context.Background()
	const key = "snapshots/snapshot.json"

	// Empty bucket: GET and HEAD report a missing snapshot.
	if _, _, err := client.GetObject(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if _, err := client.HeadObject(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// First put: no expectation, object created with a fresh ETag.
	etag, err := client.PutObject(ctx, key, []byte("v1"), "")
	if err != nil {
		t.Fatalf("put v1: %v (server said: %s)", err, fake.lastSigError)
	}
	if etag == "" {
		t.Fatal("put must return an ETag")
	}

	// Stale writer (wrong ETag) conflicts instead of clobbering.
	if _, err := client.PutObject(ctx, key, []byte("stale"), "\"bogus\""); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}

	// Fresh writer wins, then GET and HEAD observe it.
	etag2, err := client.PutObject(ctx, key, []byte("v2"), etag)
	if err != nil {
		t.Fatalf("put v2: %v (server said: %s)", err, fake.lastSigError)
	}
	body, gotETag, err := client.GetObject(ctx, key)
	if err != nil {
		t.Fatalf("get: %v (server said: %s)", err, fake.lastSigError)
	}
	if string(body) != "v2" || gotETag != etag2 {
		t.Fatalf("body %q etag %q, want v2 / %q", body, gotETag, etag2)
	}
	headETag, err := client.HeadObject(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if headETag != etag2 {
		t.Fatalf("head etag %q, want %q", headETag, etag2)
	}

	// Every request carried a verifiable SigV4 Authorization header.
	if fake.lastSigError != "" {
		t.Fatalf("signature verification: %s", fake.lastSigError)
	}
	if !strings.HasPrefix(fake.lastAuth, "AWS4-HMAC-SHA256 Credential="+testS3AccessKey+"/") {
		t.Fatalf("authorization = %q", fake.lastAuth)
	}
	if !strings.Contains(fake.lastAuth, "SignedHeaders=host;x-amz-content-sha256;x-amz-date") {
		t.Fatalf("authorization = %q", fake.lastAuth)
	}
	if !strings.Contains(fake.lastAuth, ", Signature=") {
		t.Fatalf("authorization = %q", fake.lastAuth)
	}

	// Delete clears the key again.
	if err := client.DeleteObject(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, _, err := client.GetObject(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}

	// ListObjectsV2 parses <Key> elements from the stub XML.
	keys, err := client.ListObjects(ctx, "snapshots/")
	if err != nil {
		t.Fatalf("list: %v (server said: %s)", err, fake.lastSigError)
	}
	wantKeys := []string{"snapshots/old.json", "snapshots/snapshot.json"}
	if len(keys) != len(wantKeys) {
		t.Fatalf("keys = %v, want %v", keys, wantKeys)
	}
	for i := range wantKeys {
		if keys[i] != wantKeys[i] {
			t.Fatalf("keys = %v, want %v", keys, wantKeys)
		}
	}
}

// TestS3SignsSessionToken: session tokens ride along, signed as
// x-amz-security-token and listed in SignedHeaders.
func TestS3SignsSessionToken(t *testing.T) {
	fake := newFakeS3()
	server := httptest.NewServer(fake.handler())
	defer server.Close()

	client := NewS3Client(S3Config{
		Endpoint:        server.URL,
		Region:          testS3Region,
		Bucket:          testS3Bucket,
		AccessKeyID:     testS3AccessKey,
		SecretAccessKey: testS3SecretKey,
		SessionToken:    "tok123",
		UsePathStyle:    true,
	})
	if _, err := client.HeadObject(context.Background(), "snapshots/snapshot.json"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if fake.lastToken != "tok123" {
		t.Fatalf("security token header = %q", fake.lastToken)
	}
	if !strings.Contains(fake.lastAuth, "x-amz-security-token") {
		t.Fatalf("token not signed: %q", fake.lastAuth)
	}
	if fake.lastSigError != "" {
		t.Fatalf("signature verification: %s", fake.lastSigError)
	}
}

// TestS3VirtualHostStyle builds bucket.endpoint/key URLs and signs the
// bucket-prefixed host.
func TestS3VirtualHostStyle(t *testing.T) {
	fake := newFakeS3()
	server := httptest.NewServer(fake.handler())
	defer server.Close()

	client := NewS3Client(S3Config{
		Endpoint:        server.URL,
		Region:          testS3Region,
		Bucket:          testS3Bucket,
		AccessKeyID:     testS3AccessKey,
		SecretAccessKey: testS3SecretKey,
	})
	// The bucket-prefixed hostname never resolves in a test; dial the test
	// listener directly while the request keeps its virtual-host URL and the
	// bucket-prefixed Host header that the signature must cover.
	client.client = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial(network, server.Listener.Addr().String())
			},
		},
	}
	if _, err := client.PutObject(context.Background(), "snapshots/snapshot.json", []byte("vh"), ""); err != nil {
		t.Fatalf("put: %v (server said: %s)", err, fake.lastSigError)
	}
	wantHost := testS3Bucket + "." + strings.TrimPrefix(server.URL, "http://")
	if fake.lastHost != wantHost {
		t.Fatalf("host = %q, want %q", fake.lastHost, wantHost)
	}
	if fake.lastPath != "/snapshots/snapshot.json" {
		t.Fatalf("path = %q", fake.lastPath)
	}
	if fake.lastSigError != "" {
		t.Fatalf("signature verification: %s", fake.lastSigError)
	}
}

// TestS3UnauthorizedAndForbidden maps 401 and 403 onto ErrUnauthorized.
func TestS3UnauthorizedAndForbidden(t *testing.T) {
	ctx := context.Background()
	newClient := func(serverURL string) *S3Client {
		return NewS3Client(S3Config{
			Endpoint:        serverURL,
			Region:          testS3Region,
			Bucket:          testS3Bucket,
			AccessKeyID:     testS3AccessKey,
			SecretAccessKey: testS3SecretKey,
			UsePathStyle:    true,
		})
	}

	forbidden := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer forbidden.Close()
	client := newClient(forbidden.URL)
	if _, _, err := client.GetObject(ctx, "k"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("403 get: expected ErrUnauthorized, got %v", err)
	}
	if _, err := client.PutObject(ctx, "k", []byte("x"), ""); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("403 put: expected ErrUnauthorized, got %v", err)
	}
	if err := client.DeleteObject(ctx, "k"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("403 delete: expected ErrUnauthorized, got %v", err)
	}

	unauthorized := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer unauthorized.Close()
	client = newClient(unauthorized.URL)
	if _, err := client.HeadObject(ctx, "k"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("401 head: expected ErrUnauthorized, got %v", err)
	}
}

// TestS3PutRejectsOversizedSnapshot: payloads above the 16 MiB bound fail at
// the client without touching the wire.
func TestS3PutRejectsOversizedSnapshot(t *testing.T) {
	fake := newFakeS3()
	server := httptest.NewServer(fake.handler())
	defer server.Close()

	client := NewS3Client(S3Config{
		Endpoint:        server.URL,
		Region:          testS3Region,
		Bucket:          testS3Bucket,
		AccessKeyID:     testS3AccessKey,
		SecretAccessKey: testS3SecretKey,
		UsePathStyle:    true,
	})
	oversized := make([]byte, maxSnapshotBytes+1)
	if _, err := client.PutObject(context.Background(), "snapshots/snapshot.json", oversized, ""); err == nil {
		t.Fatal("expected oversized put to fail")
	}
	if fake.hits != 0 {
		t.Fatalf("oversized put reached the server (%d hits)", fake.hits)
	}
}
