// S3 snapshot provider: minimal AWS Signature V4 client on the standard
// library only. Mirrors the WebDAV contract -- strong ETag preconditions
// (If-Match) so a stale writer surfaces as ErrConflict -- and reuses the
// package sentinels plus the 16 MiB snapshot bound.
package cloudsync

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
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// s3Service pins Signature V4 to the S3 signing namespace.
const s3Service = "s3"

// SigV4 timestamp formats: one for x-amz-date, one for the credential scope.
const (
	amzDateFormat   = "20060102T150405Z"
	dateStampFormat = "20060102"
)

// emptyPayloadSHA256 is the hex SHA-256 of the empty string. SigV4 requires it
// as x-amz-content-sha256 for bodyless requests.
var emptyPayloadSHA256 = sha256Hex(nil)

// S3Config configures one S3 snapshot bucket.
type S3Config struct {
	// Endpoint is the base URL, e.g. https://s3.us-east-1.amazonaws.com or an
	// S3-compatible server such as MinIO.
	Endpoint string
	// Region is the SigV4 region, e.g. "us-east-1".
	Region string
	// Bucket is the snapshot bucket name.
	Bucket string
	// AccessKeyID and SecretAccessKey are the static SigV4 credentials.
	AccessKeyID     string
	SecretAccessKey string
	// SessionToken is the optional STS token, signed as x-amz-security-token.
	SessionToken string
	// UsePathStyle selects endpoint/bucket/key URLs instead of the default
	// virtual-host bucket.endpoint/key style (typical for MinIO/localhost).
	UsePathStyle bool
	// Timeout bounds one HTTP round trip (default 30s).
	Timeout time.Duration
}

// S3Client talks to one S3 snapshot bucket via Signature V4.
type S3Client struct {
	config S3Config
	client *http.Client
}

// NewS3Client builds a client; a missing or non-positive Timeout defaults to
// 30 seconds.
func NewS3Client(config S3Config) *S3Client {
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &S3Client{
		config: config,
		client: &http.Client{Timeout: timeout},
	}
}

// GetObject fetches one object and its ETag. ErrNotFound when the key is
// absent; payloads above the snapshot bound are rejected.
func (c *S3Client) GetObject(ctx context.Context, key string) ([]byte, string, error) {
	response, err := c.do(ctx, http.MethodGet, key, nil, nil, nil)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()
	if err := s3StatusError("get", response.StatusCode); err != nil {
		return nil, "", err
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxSnapshotBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("s3 get body: %w", err)
	}
	if len(body) > maxSnapshotBytes {
		return nil, "", fmt.Errorf("s3 get: snapshot exceeds %d bytes", maxSnapshotBytes)
	}
	return body, response.Header.Get("ETag"), nil
}

// PutObject uploads one object. When ifMatchETag is non-empty the upload
// carries If-Match, and a 412 response maps to ErrConflict so a stale writer
// pulls and merges instead of clobbering the remote. Returns the new ETag.
func (c *S3Client) PutObject(ctx context.Context, key string, data []byte, ifMatchETag string) (string, error) {
	if int64(len(data)) > maxSnapshotBytes {
		return "", fmt.Errorf("s3 put: snapshot exceeds %d bytes", maxSnapshotBytes)
	}
	extraHeaders := map[string]string{}
	if ifMatchETag != "" {
		extraHeaders["If-Match"] = ifMatchETag
	}
	response, err := c.do(ctx, http.MethodPut, key, nil, data, extraHeaders)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if err := s3StatusError("put", response.StatusCode); err != nil {
		return "", err
	}
	return response.Header.Get("ETag"), nil
}

// DeleteObject removes one object. ErrNotFound when the key is absent.
func (c *S3Client) DeleteObject(ctx context.Context, key string) error {
	response, err := c.do(ctx, http.MethodDelete, key, nil, nil, nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return s3StatusError("delete", response.StatusCode)
}

// HeadObject returns the ETag of one object. ErrNotFound when absent.
func (c *S3Client) HeadObject(ctx context.Context, key string) (string, error) {
	response, err := c.do(ctx, http.MethodHead, key, nil, nil, nil)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if err := s3StatusError("head", response.StatusCode); err != nil {
		return "", err
	}
	return response.Header.Get("ETag"), nil
}

// ListObjects lists object keys under prefix via ListObjectsV2, following
// continuation tokens until the listing is complete.
func (c *S3Client) ListObjects(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	token := ""
	for {
		query := url.Values{}
		query.Set("list-type", "2")
		if prefix != "" {
			query.Set("prefix", prefix)
		}
		if token != "" {
			query.Set("continuation-token", token)
		}
		response, err := c.do(ctx, http.MethodGet, "", query, nil, nil)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, maxSnapshotBytes+1))
		response.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("s3 list body: %w", err)
		}
		if err := s3StatusError("list", response.StatusCode); err != nil {
			return nil, err
		}
		var parsed listObjectsV2Result
		if err := xml.Unmarshal(data, &parsed); err != nil {
			return nil, fmt.Errorf("s3 list xml: %w", err)
		}
		for _, item := range parsed.Contents {
			keys = append(keys, item.Key)
		}
		if !parsed.IsTruncated || parsed.NextContinuationToken == "" {
			return keys, nil
		}
		token = parsed.NextContinuationToken
	}
}

type listObjectsV2Result struct {
	IsTruncated           bool   `xml:"IsTruncated"`
	NextContinuationToken string `xml:"NextContinuationToken"`
	Contents              []struct {
		Key string `xml:"Key"`
	} `xml:"Contents"`
}

// do builds the object URL, signs the request and sends it. The payload hash
// is the real body hash for PUT and the empty-body hash otherwise.
func (c *S3Client) do(ctx context.Context, method, key string, query url.Values, body []byte, extraHeaders map[string]string) (*http.Response, error) {
	rawURL, err := c.objectURL(key, query)
	if err != nil {
		return nil, err
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return nil, err
	}
	for name, value := range extraHeaders {
		request.Header.Set(name, value)
	}
	payloadHash := emptyPayloadSHA256
	if method == http.MethodPut {
		payloadHash = sha256Hex(body)
	}
	c.sign(request, payloadHash, time.Now())
	response, err := c.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("s3 %s: %w", strings.ToLower(method), err)
	}
	return response, nil
}

// objectURL builds the request URL for one object key plus query, returning
// both the path-style and virtual-host shapes. The key is percent-encoded per
// RFC 3986 so the wire path matches the canonical request exactly.
func (c *S3Client) objectURL(key string, query url.Values) (string, error) {
	if c.config.Endpoint == "" {
		return "", errors.New("s3: endpoint is required")
	}
	if c.config.Bucket == "" {
		return "", errors.New("s3: bucket is required")
	}
	base := strings.TrimRight(c.config.Endpoint, "/")
	escapedKey := escapeObjectKey(key)
	bucket := awsURIEncode(c.config.Bucket, false)
	var raw string
	if c.config.UsePathStyle {
		raw = base + "/" + bucket + "/" + escapedKey
	} else {
		parsed, err := url.Parse(base)
		if err != nil {
			return "", fmt.Errorf("s3: bad endpoint: %w", err)
		}
		raw = parsed.Scheme + "://" + bucket + "." + parsed.Host + "/" + escapedKey
	}
	if q := canonicalQueryString(query); q != "" {
		raw += "?" + q
	}
	return raw, nil
}

// sign applies Signature V4 to the request in place. The payload hash must be
// the hex SHA-256 of the exact bytes being sent (empty-body hash when none).
func (c *S3Client) sign(request *http.Request, payloadHash string, now time.Time) {
	stamp := now.UTC()
	amzDate := stamp.Format(amzDateFormat)
	dateStamp := stamp.Format(dateStampFormat)
	request.Header.Set("x-amz-date", amzDate)
	request.Header.Set("x-amz-content-sha256", payloadHash)
	if c.config.SessionToken != "" {
		request.Header.Set("x-amz-security-token", c.config.SessionToken)
	}
	// Host participates in the canonical request. Go sends request.URL.Host
	// regardless of Header["Host"], so this write is canonicalization only.
	request.Header.Set("Host", request.URL.Host)

	scope := strings.Join([]string{dateStamp, c.config.Region, s3Service, "aws4_request"}, "/")
	canonical, signedHeaders := canonicalRequest(request.Method, request.URL.EscapedPath(), request.URL.RawQuery, request.Header, payloadHash)
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + sha256Hex([]byte(canonical))
	key := signingKey(c.config.SecretAccessKey, dateStamp, c.config.Region, s3Service)
	signature := hex.EncodeToString(hmacSHA256(key, []byte(stringToSign)))
	request.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+c.config.AccessKeyID+"/"+scope+", SignedHeaders="+signedHeaders+", Signature="+signature)
}

// signingKey derives the SigV4 signing key: an HMAC-SHA256 chain over the
// date, region and service, rooted at "AWS4"+secret (AWS-documented scheme).
func signingKey(secret, date, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(date))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	return hmacSHA256(kService, []byte("aws4_request"))
}

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// canonicalRequest builds the SigV4 canonical request string and its sorted
// signed-headers list. canonicalHeaders already ends in "\n", so joining
// produces the blank line before the signed-headers list per the AWS spec.
func canonicalRequest(method, canonicalURI, canonicalQuery string, headers http.Header, payloadHash string) (string, string) {
	canonicalHeaders, signedHeaders := canonicalHeaders(headers)
	lines := []string{method, canonicalURI, canonicalQuery, canonicalHeaders, signedHeaders, payloadHash}
	return strings.Join(lines, "\n"), signedHeaders
}

// canonicalHeaders renders each header as "lowercase-name:trimmed-value\n",
// sorted by name, and returns the ";"-joined signed-headers list.
func canonicalHeaders(headers http.Header) (string, string) {
	names := make([]string, 0, len(headers))
	original := make(map[string]string, len(headers))
	for name := range headers {
		lower := strings.ToLower(name)
		names = append(names, lower)
		original[lower] = name
	}
	sort.Strings(names)
	var builder strings.Builder
	for _, lower := range names {
		values := headers[original[lower]]
		trimmed := make([]string, len(values))
		for i, value := range values {
			trimmed[i] = strings.Join(strings.Fields(value), " ")
		}
		builder.WriteString(lower)
		builder.WriteString(":")
		builder.WriteString(strings.Join(trimmed, ","))
		builder.WriteString("\n")
	}
	return builder.String(), strings.Join(names, ";")
}

// canonicalQueryString renders query parameters sorted by encoded key (then
// value), with keys and values strictly percent-encoded per the SigV4 spec.
func canonicalQueryString(query url.Values) string {
	if len(query) == 0 {
		return ""
	}
	type pair struct{ key, value string }
	pairs := make([]pair, 0, len(query))
	for key, values := range query {
		for _, value := range values {
			pairs = append(pairs, pair{awsURIEncode(key, true), awsURIEncode(value, true)})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].key != pairs[j].key {
			return pairs[i].key < pairs[j].key
		}
		return pairs[i].value < pairs[j].value
	})
	parts := make([]string, len(pairs))
	for i, p := range pairs {
		parts[i] = p.key + "=" + p.value
	}
	return strings.Join(parts, "&")
}

// awsURIEncode percent-encodes s as SigV4 requires: unreserved characters
// survive, everything else becomes uppercase %XX. Slashes are kept in object
// paths (encodeSlash false) and encoded in query components (true).
func awsURIEncode(s string, encodeSlash bool) string {
	var builder strings.Builder
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if isURIUnreserved(ch) || (ch == '/' && !encodeSlash) {
			builder.WriteByte(ch)
			continue
		}
		fmt.Fprintf(&builder, "%%%02X", ch)
	}
	return builder.String()
}

func isURIUnreserved(ch byte) bool {
	switch {
	case ch >= 'A' && ch <= 'Z', ch >= 'a' && ch <= 'z', ch >= '0' && ch <= '9':
		return true
	case ch == '-' || ch == '_' || ch == '.' || ch == '~':
		return true
	}
	return false
}

// escapeObjectKey percent-encodes each slash-separated segment of the key,
// preserving the slashes as S3 requires.
func escapeObjectKey(key string) string {
	key = strings.TrimLeft(key, "/")
	segments := strings.Split(key, "/")
	for i, segment := range segments {
		segments[i] = awsURIEncode(segment, false)
	}
	return strings.Join(segments, "/")
}

// s3StatusError maps an HTTP status onto the package sentinels, mirroring the
// WebDAV provider: 404 not found, 401/403 unauthorized, 412 conflict, 2xx ok.
func s3StatusError(op string, status int) error {
	switch status {
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrUnauthorized
	case http.StatusPreconditionFailed:
		return ErrConflict
	}
	if status >= 200 && status < 300 {
		return nil
	}
	return fmt.Errorf("s3 %s: unexpected status %d", op, status)
}
