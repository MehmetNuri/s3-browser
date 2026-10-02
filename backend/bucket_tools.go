package main

import (
	"errors"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// IncompleteUpload is a multipart upload that was started but never
// completed or aborted. Its parts are stored, and billed, without being
// visible as an object.
type IncompleteUpload struct {
	Key       string `json:"key"`
	UploadID  string `json:"uploadId"`
	Initiated string `json:"initiated"`
	Size      int64  `json:"size"` // stored parts; -1 when it could not be read
	Stale     bool   `json:"stale"`
}

const (
	incompleteUploadLimit = 1000
	// Uploads younger than this may still be running, here or elsewhere.
	staleUploadAge = 24 * time.Hour
	// Part sizes need one more request per upload.
	uploadSizeLimit = 100
)

type pendingUpload struct {
	key, id   string
	initiated time.Time
}

func (a *App) pendingUploads(c *s3.Client, bucket string) ([]pendingUpload, error) {
	var uploads []pendingUpload
	p := s3.NewListMultipartUploadsPaginator(c, &s3.ListMultipartUploadsInput{Bucket: aws.String(bucket)})
	for p.HasMorePages() && len(uploads) < incompleteUploadLimit {
		out, err := p.NextPage(a.ctx)
		if err != nil {
			return nil, describeErr(err)
		}
		for _, u := range out.Uploads {
			uploads = append(uploads, pendingUpload{aws.ToString(u.Key), aws.ToString(u.UploadId), aws.ToTime(u.Initiated)})
		}
	}
	sort.Slice(uploads, func(i, j int) bool { return uploads[i].initiated.Before(uploads[j].initiated) })
	return uploads, nil
}

// ListIncompleteUploads lists unfinished multipart uploads of a bucket, oldest first.
func (a *App) ListIncompleteUploads(bucket string) ([]IncompleteUpload, error) {
	c, err := a.cli()
	if err != nil {
		return nil, err
	}
	uploads, err := a.pendingUploads(c, bucket)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	result := make([]IncompleteUpload, len(uploads))
	for i, u := range uploads {
		result[i] = IncompleteUpload{Key: u.key, UploadID: u.id, Initiated: fmtTime(&u.initiated), Size: -1, Stale: now.Sub(u.initiated) > staleUploadAge}
		if i >= uploadSizeLimit {
			continue
		}
		var size int64
		parts := s3.NewListPartsPaginator(c, &s3.ListPartsInput{Bucket: aws.String(bucket), Key: aws.String(u.key), UploadId: aws.String(u.id)})
		failed := false
		for parts.HasMorePages() {
			out, err := parts.NextPage(a.ctx)
			if err != nil {
				failed = true
				break
			}
			for _, part := range out.Parts {
				size += aws.ToInt64(part.Size)
			}
		}
		if !failed {
			result[i].Size = size
		}
	}
	return result, nil
}

// AbortStaleUploads aborts the unfinished uploads that are older than a day
// and returns how many were removed. Recent uploads are kept because they
// may still be in progress.
func (a *App) AbortStaleUploads(bucket string) (int, error) {
	c, err := a.cli()
	if err != nil {
		return 0, err
	}
	uploads, err := a.pendingUploads(c, bucket)
	if err != nil {
		return 0, err
	}
	aborted := 0
	now := time.Now()
	for _, u := range uploads {
		if now.Sub(u.initiated) <= staleUploadAge {
			continue
		}
		if _, err := c.AbortMultipartUpload(a.ctx, &s3.AbortMultipartUploadInput{Bucket: aws.String(bucket), Key: aws.String(u.key), UploadId: aws.String(u.id)}); err != nil {
			return aborted, describeErr(err)
		}
		aborted++
	}
	return aborted, nil
}

// BucketVersioning returns "Enabled", "Suspended" or "" when it was never enabled.
func (a *App) BucketVersioning(bucket string) (string, error) {
	c, err := a.cli()
	if err != nil {
		return "", err
	}
	out, err := c.GetBucketVersioning(a.ctx, &s3.GetBucketVersioningInput{Bucket: aws.String(bucket)})
	if err != nil {
		return "", describeErr(err)
	}
	return string(out.Status), nil
}

// SetBucketVersioning enables or suspends versioning. Suspending keeps the
// versions that already exist.
func (a *App) SetBucketVersioning(bucket string, enabled bool) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	if bucket == "" {
		return errors.New(T("bucketRequired"))
	}
	status := types.BucketVersioningStatusSuspended
	if enabled {
		status = types.BucketVersioningStatusEnabled
	}
	_, err = c.PutBucketVersioning(a.ctx, &s3.PutBucketVersioningInput{
		Bucket: aws.String(bucket), VersioningConfiguration: &types.VersioningConfiguration{Status: status},
	})
	return describeErr(err)
}
