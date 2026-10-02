package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// CapResult is the outcome of a single capability probe.
type CapResult struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Name     string `json:"name"`
	Status   string `json:"status"` // ok | unsupported | denied | error | skipped
	Detail   string `json:"detail"`
	Ms       int64  `json:"ms"`
}

type CapOptions struct {
	Bucket        string `json:"bucket"`
	IncludeBucket bool   `json:"includeBucket"` // create/delete a temporary bucket
}

// Config errors meaning "the API exists, there's just no config set".
var configMissing = map[string]bool{
	"NoSuchCORSConfiguration":                        true,
	"NoSuchBucketPolicy":                             true,
	"NoSuchLifecycleConfiguration":                   true,
	"ServerSideEncryptionConfigurationNotFoundError": true,
	"NoSuchTagSet":                                   true,
	"NoSuchTagSetError":                              true,
	"NoSuchWebsiteConfiguration":                     true,
	"ObjectLockConfigurationNotFoundError":           true,
	"OwnershipControlsNotFoundError":                 true,
	"ReplicationConfigurationNotFoundError":          true,
	"NoSuchPublicAccessBlockConfiguration":           true,
}

// errUnsupported marks a call that "succeeded" but was silently ignored.
type errUnsupported string

func (e errUnsupported) Error() string { return string(e) }

func classify(err error) (status, detail string) {
	if err == nil {
		return "ok", ""
	}
	var eu errUnsupported
	if errors.As(err, &eu) {
		return "unsupported", string(eu)
	}
	code, msg, sc := "", err.Error(), 0
	var ae smithy.APIError
	if errors.As(err, &ae) {
		code, msg = ae.ErrorCode(), ae.ErrorMessage()
	}
	var re *awshttp.ResponseError
	if errors.As(err, &re) {
		sc = re.HTTPStatusCode()
	}
	detail = strings.TrimSpace(fmt.Sprintf("%s %s", code, msg))
	if sc != 0 {
		detail = fmt.Sprintf("HTTP %d · %s", sc, detail)
	}
	lm := strings.ToLower(msg + " " + code)
	switch {
	case configMissing[code]:
		return "ok", T("configMissing", code)
	case code == "NotImplemented" || code == "NotSupported" || code == "MethodNotAllowed" ||
		code == "FeatureNotEnabled" || code == "InvalidKey" ||
		sc == 501 || sc == 405 ||
		strings.Contains(lm, "not implemented") || strings.Contains(lm, "not supported") || strings.Contains(lm, "unsupported"):
		return "unsupported", detail
	case code == "AccessDenied" || code == "Forbidden" || sc == 403:
		return "denied", detail
	case sc == 404 && !strings.HasPrefix(code, "NoSuchBucket") && !strings.HasPrefix(code, "NoSuchKey"):
		return "unsupported", detail
	}
	return "error", detail
}

func describeErr(err error) error {
	if err == nil {
		return nil
	}
	var ae smithy.APIError
	if errors.As(err, &ae) && ae.ErrorCode() == "EntityTooLarge" {
		return errors.New(T("entityTooLarge"))
	}
	_, d := classify(err)
	if d == "" {
		return err
	}
	return errors.New(d)
}

func urlPathEscape(s string) string { return url.PathEscape(s) }

type capRunner struct {
	a       *App
	ctx     context.Context
	results []CapResult
}

func (r *capRunner) run(cat, name string, fn func(ctx context.Context) (string, error)) bool {
	ctx, cancel := context.WithTimeout(r.ctx, 30*time.Second)
	defer cancel()
	start := time.Now()
	info, err := fn(ctx)
	status, detail := classify(err)
	if err == nil {
		detail = info
	}
	res := CapResult{ID: fmt.Sprintf("%d", len(r.results)), Category: cat, Name: name, Status: status, Detail: detail, Ms: time.Since(start).Milliseconds()}
	r.results = append(r.results, res)
	r.a.emitEvent("cap", res)
	return err == nil
}

func (r *capRunner) skip(cat, name, why string) {
	res := CapResult{ID: fmt.Sprintf("%d", len(r.results)), Category: cat, Name: name, Status: "skipped", Detail: why}
	r.results = append(r.results, res)
	r.a.emitEvent("cap", res)
}

// RunCapabilities probes which S3 APIs the connected endpoint supports.
// It writes and cleans up temporary objects under ".s3browser-captest/".
func (a *App) RunCapabilities(opt CapOptions) ([]CapResult, error) {
	c, err := a.cli()
	if err != nil {
		return nil, err
	}
	a.mu.RLock()
	skipTLS := a.profile.SkipTLS
	a.mu.RUnlock()
	r := &capRunner{a: a, ctx: a.ctx}
	b := aws.String(opt.Bucket)
	var (
		svc = T("catService")
		bkt = T("catBucket")
		obj = T("catObject")
		mp  = T("catMultipart")
		ps  = T("catPresigned")
		cfg = T("catConfig")
	)

	r.run(svc, "ListBuckets", func(ctx context.Context) (string, error) {
		out, err := c.ListBuckets(ctx, &s3.ListBucketsInput{})
		if err != nil {
			return "", err
		}
		return T("nBuckets", len(out.Buckets)), nil
	})

	if opt.IncludeBucket {
		tmp := "s3b-cap-" + randID()
		if r.run(svc, "CreateBucket", func(ctx context.Context) (string, error) {
			_, err := c.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(tmp)})
			return tmp, err
		}) {
			r.run(svc, "DeleteBucket", func(ctx context.Context) (string, error) {
				_, err := c.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(tmp)})
				return tmp, err
			})
		} else {
			r.skip(svc, "DeleteBucket", T("failed", "CreateBucket"))
		}
	}

	if opt.Bucket == "" {
		return r.results, nil
	}

	r.run(bkt, "HeadBucket", func(ctx context.Context) (string, error) {
		_, err := c.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: b})
		return "", err
	})
	r.run(bkt, "GetBucketLocation", func(ctx context.Context) (string, error) {
		out, err := c.GetBucketLocation(ctx, &s3.GetBucketLocationInput{Bucket: b})
		if err != nil {
			return "", err
		}
		loc := string(out.LocationConstraint)
		if loc == "" {
			loc = T("locEmpty")
		}
		return loc, nil
	})
	r.run(bkt, "ListObjectsV2", func(ctx context.Context) (string, error) {
		out, err := c.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: b, MaxKeys: aws.Int32(5), Delimiter: aws.String("/")})
		if err != nil {
			return "", err
		}
		return T("objsPrefixes", len(out.Contents), len(out.CommonPrefixes)), nil
	})
	r.run(bkt, "ListObjects (v1)", func(ctx context.Context) (string, error) {
		_, err := c.ListObjects(ctx, &s3.ListObjectsInput{Bucket: b, MaxKeys: aws.Int32(5)})
		return "", err
	})
	r.run(bkt, "ListObjectVersions", func(ctx context.Context) (string, error) {
		_, err := c.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{Bucket: b, MaxKeys: aws.Int32(5)})
		return "", err
	})
	r.run(bkt, "ListMultipartUploads", func(ctx context.Context) (string, error) {
		out, err := c.ListMultipartUploads(ctx, &s3.ListMultipartUploadsInput{Bucket: b})
		if err != nil {
			return "", err
		}
		return T("openUploads", len(out.Uploads)), nil
	})

	cfgProbe := func(name string, fn func(ctx context.Context) error) {
		r.run(cfg, name, func(ctx context.Context) (string, error) { return "", fn(ctx) })
	}
	cfgProbe("GetBucketVersioning", func(ctx context.Context) error {
		_, err := c.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{Bucket: b})
		return err
	})
	cfgProbe("GetBucketCors", func(ctx context.Context) error {
		_, err := c.GetBucketCors(ctx, &s3.GetBucketCorsInput{Bucket: b})
		return err
	})
	cfgProbe("GetBucketPolicy", func(ctx context.Context) error {
		_, err := c.GetBucketPolicy(ctx, &s3.GetBucketPolicyInput{Bucket: b})
		return err
	})
	cfgProbe("GetBucketLifecycleConfiguration", func(ctx context.Context) error {
		_, err := c.GetBucketLifecycleConfiguration(ctx, &s3.GetBucketLifecycleConfigurationInput{Bucket: b})
		return err
	})
	cfgProbe("GetBucketEncryption", func(ctx context.Context) error {
		_, err := c.GetBucketEncryption(ctx, &s3.GetBucketEncryptionInput{Bucket: b})
		return err
	})
	cfgProbe("GetBucketTagging", func(ctx context.Context) error {
		_, err := c.GetBucketTagging(ctx, &s3.GetBucketTaggingInput{Bucket: b})
		return err
	})
	cfgProbe("GetBucketAcl", func(ctx context.Context) error {
		_, err := c.GetBucketAcl(ctx, &s3.GetBucketAclInput{Bucket: b})
		return err
	})
	cfgProbe("GetObjectLockConfiguration", func(ctx context.Context) error {
		_, err := c.GetObjectLockConfiguration(ctx, &s3.GetObjectLockConfigurationInput{Bucket: b})
		return err
	})

	base := ".s3browser-captest/" + randID() + "/"
	key := base + "obj.txt"
	body := []byte("hello from s3browser capability test")
	var cleanup []string

	put := r.run(obj, "PutObject (metadata + content-type)", func(ctx context.Context) (string, error) {
		_, err := c.PutObject(ctx, &s3.PutObjectInput{
			Bucket: b, Key: aws.String(key), Body: bytes.NewReader(body),
			ContentType: aws.String("text/plain"),
			Metadata:    map[string]string{"s3b-test": "yes"},
		})
		return key, err
	})
	if put {
		cleanup = append(cleanup, key)
		r.run(obj, "HeadObject", func(ctx context.Context) (string, error) {
			out, err := c.HeadObject(ctx, &s3.HeadObjectInput{Bucket: b, Key: aws.String(key)})
			if err != nil {
				return "", err
			}
			meta := T("metaLost")
			if out.Metadata["s3b-test"] == "yes" {
				meta = T("metaKept")
			}
			return T("headInfo", aws.ToInt64(out.ContentLength), aws.ToString(out.ContentType), meta), nil
		})
		r.run(obj, "GetObject", func(ctx context.Context) (string, error) {
			out, err := c.GetObject(ctx, &s3.GetObjectInput{Bucket: b, Key: aws.String(key)})
			if err != nil {
				return "", err
			}
			defer out.Body.Close()
			got, _ := io.ReadAll(io.LimitReader(out.Body, 1024))
			if !bytes.Equal(got, body) {
				return "", errors.New(T("contentMismatch"))
			}
			return T("contentOK"), nil
		})
		r.run(obj, "GetObject Range", func(ctx context.Context) (string, error) {
			out, err := c.GetObject(ctx, &s3.GetObjectInput{Bucket: b, Key: aws.String(key), Range: aws.String("bytes=0-4")})
			if err != nil {
				return "", err
			}
			defer out.Body.Close()
			got, _ := io.ReadAll(io.LimitReader(out.Body, 1024))
			if string(got) != "hello" {
				return "", errors.New(T("rangeIgnored", len(got)))
			}
			return "bytes=0-4 → \"hello\"", nil
		})
		r.run(obj, "GetObject If-None-Match (304)", func(ctx context.Context) (string, error) {
			h, err := c.HeadObject(ctx, &s3.HeadObjectInput{Bucket: b, Key: aws.String(key)})
			if err != nil {
				return "", err
			}
			_, err = c.GetObject(ctx, &s3.GetObjectInput{Bucket: b, Key: aws.String(key), IfNoneMatch: h.ETag})
			var re *awshttp.ResponseError
			if errors.As(err, &re) && re.HTTPStatusCode() == 304 {
				return "304 Not Modified", nil
			}
			if err == nil {
				return "", errors.New(T("condIgnored"))
			}
			return "", err
		})
		r.run(obj, "CopyObject", func(ctx context.Context) (string, error) {
			dst := base + "copy.txt"
			_, err := c.CopyObject(ctx, &s3.CopyObjectInput{Bucket: b, Key: aws.String(dst), CopySource: aws.String(copySource(opt.Bucket, key))})
			if err == nil {
				cleanup = append(cleanup, dst)
			}
			return dst, err
		})
		r.run(obj, "PutObjectTagging", func(ctx context.Context) (string, error) {
			_, err := c.PutObjectTagging(ctx, &s3.PutObjectTaggingInput{Bucket: b, Key: aws.String(key),
				Tagging: &types.Tagging{TagSet: []types.Tag{{Key: aws.String("k"), Value: aws.String("v")}}}})
			return "", err
		})
		r.run(obj, "GetObjectTagging", func(ctx context.Context) (string, error) {
			out, err := c.GetObjectTagging(ctx, &s3.GetObjectTaggingInput{Bucket: b, Key: aws.String(key)})
			if err != nil {
				return "", err
			}
			if len(out.TagSet) == 0 {
				return "", errUnsupported(T("tagsIgnored"))
			}
			return T("nTags", len(out.TagSet)), nil
		})
		r.run(obj, "GetObjectAcl", func(ctx context.Context) (string, error) {
			_, err := c.GetObjectAcl(ctx, &s3.GetObjectAclInput{Bucket: b, Key: aws.String(key)})
			return "", err
		})

		pc := s3.NewPresignClient(c)
		r.run(ps, "Presigned GET", func(ctx context.Context) (string, error) {
			req, err := pc.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: b, Key: aws.String(key)}, s3.WithPresignExpires(5*time.Minute))
			if err != nil {
				return "", err
			}
			return doPresigned(ctx, skipTLS, req.Method, req.URL, req.SignedHeader, nil)
		})
		r.run(ps, "Presigned PUT", func(ctx context.Context) (string, error) {
			k := base + "presigned.txt"
			req, err := pc.PresignPutObject(ctx, &s3.PutObjectInput{Bucket: b, Key: aws.String(k)}, s3.WithPresignExpires(5*time.Minute))
			if err != nil {
				return "", err
			}
			res, err := doPresigned(ctx, skipTLS, req.Method, req.URL, req.SignedHeader, []byte("presigned"))
			if err == nil {
				cleanup = append(cleanup, k)
			}
			return res, err
		})
	} else {
		for _, n := range []string{"HeadObject", "GetObject", "GetObject Range", "CopyObject", T("objectTagging"), "Presigned URL"} {
			r.skip(obj, n, T("failed", "PutObject"))
		}
	}

	keyProbe := func(name, k string) {
		r.run(obj, name, func(ctx context.Context) (string, error) {
			if _, err := c.PutObject(ctx, &s3.PutObjectInput{Bucket: b, Key: aws.String(k), Body: strings.NewReader("x")}); err != nil {
				return "", err
			}
			cleanup = append(cleanup, k)
			_, err := c.HeadObject(ctx, &s3.HeadObjectInput{Bucket: b, Key: aws.String(k)})
			return strings.TrimPrefix(k, base), err
		})
	}
	keyProbe(T("keySpace"), base+"with space.txt")
	keyProbe(T("keySpecial"), base+"a+b&c=d,e.txt")
	keyProbe(T("keyUnicode"), base+"ünicode-şğı.txt")

	mkey := base + "multipart.bin"
	var uploadID *string
	created := r.run(mp, "CreateMultipartUpload", func(ctx context.Context) (string, error) {
		out, err := c.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{Bucket: b, Key: aws.String(mkey)})
		if err != nil {
			return "", err
		}
		uploadID = out.UploadId
		return "", nil
	})
	if created {
		var etag *string
		r.run(mp, "UploadPart", func(ctx context.Context) (string, error) {
			out, err := c.UploadPart(ctx, &s3.UploadPartInput{Bucket: b, Key: aws.String(mkey), UploadId: uploadID,
				PartNumber: aws.Int32(1), Body: bytes.NewReader(bytes.Repeat([]byte("m"), 1024))})
			if err != nil {
				return "", err
			}
			etag = out.ETag
			return "1 KB part", nil
		})
		r.run(mp, "ListParts", func(ctx context.Context) (string, error) {
			out, err := c.ListParts(ctx, &s3.ListPartsInput{Bucket: b, Key: aws.String(mkey), UploadId: uploadID})
			if err != nil {
				return "", err
			}
			return T("nParts", len(out.Parts)), nil
		})
		if etag != nil {
			ok := r.run(mp, "CompleteMultipartUpload", func(ctx context.Context) (string, error) {
				_, err := c.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{Bucket: b, Key: aws.String(mkey), UploadId: uploadID,
					MultipartUpload: &types.CompletedMultipartUpload{Parts: []types.CompletedPart{{ETag: etag, PartNumber: aws.Int32(1)}}}})
				return "", err
			})
			if ok {
				cleanup = append(cleanup, mkey)
			} else {
				_, _ = c.AbortMultipartUpload(r.ctx, &s3.AbortMultipartUploadInput{Bucket: b, Key: aws.String(mkey), UploadId: uploadID})
			}
		} else {
			r.skip(mp, "CompleteMultipartUpload", T("failed", "UploadPart"))
			_, _ = c.AbortMultipartUpload(r.ctx, &s3.AbortMultipartUploadInput{Bucket: b, Key: aws.String(mkey), UploadId: uploadID})
		}
		r.run(mp, "AbortMultipartUpload", func(ctx context.Context) (string, error) {
			k := base + "aborted.bin"
			out, err := c.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{Bucket: b, Key: aws.String(k)})
			if err != nil {
				return "", err
			}
			_, err = c.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{Bucket: b, Key: aws.String(k), UploadId: out.UploadId})
			return "", err
		})
	} else {
		for _, n := range []string{"UploadPart", "ListParts", "CompleteMultipartUpload", "AbortMultipartUpload"} {
			r.skip(mp, n, T("failed", "CreateMultipartUpload"))
		}
	}

	if len(cleanup) > 1 {
		batch := cleanup[1:]
		r.run(obj, "DeleteObjects (batch)", func(ctx context.Context) (string, error) {
			ids := make([]types.ObjectIdentifier, len(batch))
			for i, k := range batch {
				ids[i] = types.ObjectIdentifier{Key: aws.String(k)}
			}
			out, err := c.DeleteObjects(ctx, &s3.DeleteObjectsInput{Bucket: b, Delete: &types.Delete{Objects: ids}})
			if err != nil {
				for _, k := range batch { // fallback cleanup
					_, _ = c.DeleteObject(r.ctx, &s3.DeleteObjectInput{Bucket: b, Key: aws.String(k)})
				}
				return "", err
			}
			if len(out.Errors) > 0 {
				return "", errors.New(T("nErrors", len(out.Errors), aws.ToString(out.Errors[0].Message)))
			}
			return T("nDeleted", len(out.Deleted)), nil
		})
	}
	if len(cleanup) > 0 {
		r.run(obj, "DeleteObject", func(ctx context.Context) (string, error) {
			_, err := c.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: b, Key: aws.String(cleanup[0])})
			return "", err
		})
	}
	return r.results, nil
}

func doPresigned(ctx context.Context, skipTLS bool, method, u string, hdr http.Header, body []byte) (string, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return "", err
	}
	for k, v := range hdr {
		if strings.EqualFold(k, "host") {
			continue
		}
		req.Header[k] = v
	}
	if body != nil {
		req.ContentLength = int64(len(body))
	}
	client := &http.Client{Timeout: 30 * time.Second}
	if skipTLS {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return fmt.Sprintf("HTTP %d", resp.StatusCode), nil
}
