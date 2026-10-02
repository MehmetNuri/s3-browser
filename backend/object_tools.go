package main

import (
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// The encoded image must fit in one desktop protocol message.
const imagePreviewLimit = 4 << 20

// Formats decoded by the renderer's <img>. SVG is excluded: it is a document
// format and is detected as text rather than as an image.
var previewImageTypes = map[string]bool{
	"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true, "image/bmp": true,
}

// PreviewImage returns a small image object as a data URL. The type comes
// from the bytes themselves, not from the object's stored Content-Type.
func (a *App) PreviewImage(bucket, key string) (string, error) {
	c, err := a.cli()
	if err != nil {
		return "", err
	}
	out, err := c.GetObject(a.ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return "", describeErr(err)
	}
	defer out.Body.Close()
	if aws.ToInt64(out.ContentLength) > imagePreviewLimit {
		return "", errors.New(T("previewTooLarge"))
	}
	data, err := io.ReadAll(io.LimitReader(out.Body, imagePreviewLimit+1))
	if err != nil {
		return "", err
	}
	if len(data) > imagePreviewLimit {
		return "", errors.New(T("previewTooLarge"))
	}
	contentType := http.DetectContentType(data)
	if !previewImageTypes[contentType] {
		return "", errors.New(T("previewUnsupported"))
	}
	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

// Listing stops here so a huge prefix cannot keep the backend busy for hours.
const folderStatsLimit = 100_000

type FolderStats struct {
	Objects   int64 `json:"objects"`
	Size      int64 `json:"size"`
	Truncated bool  `json:"truncated"` // more objects exist than were counted
}

// FolderStats counts the objects under a prefix and adds up their sizes.
func (a *App) FolderStats(bucket, prefix string) (FolderStats, error) {
	c, err := a.cli()
	if err != nil {
		return FolderStats{}, err
	}
	var stats FolderStats
	p := s3.NewListObjectsV2Paginator(c, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(prefix)})
	for p.HasMorePages() {
		out, err := p.NextPage(a.ctx)
		if err != nil {
			return FolderStats{}, describeErr(err)
		}
		for _, o := range out.Contents {
			if aws.ToString(o.Key) == prefix { // folder placeholder
				continue
			}
			stats.Objects++
			stats.Size += aws.ToInt64(o.Size)
		}
		if stats.Objects >= folderStatsLimit && p.HasMorePages() {
			stats.Truncated = true
			break
		}
	}
	return stats, nil
}

// SaveText replaces a small text object with edited content, keeping its
// headers and metadata. Only objects that the preview shows completely can be
// saved, so a truncated preview can never overwrite a larger object.
func (a *App) SaveText(bucket, key, content, etag string) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	head, err := c.HeadObject(a.ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return describeErr(err)
	}
	if aws.ToInt64(head.ContentLength) > previewLimit || len(content) > 1<<20 || aws.ToString(head.ContentEncoding) != "" {
		return errors.New(T("editTooLarge"))
	}
	// The object was replaced by someone else since it was opened.
	if etag != "" && strings.Trim(aws.ToString(head.ETag), `"`) != etag {
		return errors.New(T("objectChanged"))
	}
	contentType := aws.ToString(head.ContentType)
	if contentType == "" {
		contentType = "text/plain; charset=utf-8"
	}
	_, err = c.PutObject(a.ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket), Key: aws.String(key), Body: strings.NewReader(content),
		ContentType: aws.String(contentType), CacheControl: head.CacheControl,
		ContentDisposition: head.ContentDisposition, Metadata: head.Metadata,
	})
	return describeErr(err)
}
