package main

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// CopyObject is limited to 5 GB per request; larger objects are copied
// server-side in parts, which also keeps every request short.
const (
	singleCopyLimit = 5 << 30
	copyPartSize    = 512 << 20 // 10 000 parts cover 5 TB
)

// serverCopy copies an object within the service. For objects above the
// single-request limit it runs a multipart copy with the same metadata,
// storage class and encryption as the CopyObjectInput describes.
func serverCopy(ctx context.Context, c *s3.Client, in *s3.CopyObjectInput, size int64) error {
	if size <= singleCopyLimit {
		_, err := c.CopyObject(ctx, in)
		return err
	}
	create, err := c.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket: in.Bucket, Key: in.Key,
		Metadata: in.Metadata, ContentType: in.ContentType, CacheControl: in.CacheControl,
		ContentDisposition: in.ContentDisposition, ContentEncoding: in.ContentEncoding,
		StorageClass: in.StorageClass, ServerSideEncryption: in.ServerSideEncryption,
		SSEKMSKeyId: in.SSEKMSKeyId, BucketKeyEnabled: in.BucketKeyEnabled, Tagging: in.Tagging,
	})
	if err != nil {
		return err
	}
	uploadID := create.UploadId
	abort := func() {
		_, _ = c.AbortMultipartUpload(context.WithoutCancel(ctx), &s3.AbortMultipartUploadInput{Bucket: in.Bucket, Key: in.Key, UploadId: uploadID})
	}
	var parts []types.CompletedPart
	for start, number := int64(0), int32(1); start < size; start, number = start+copyPartSize, number+1 {
		end := min(start+copyPartSize, size) - 1
		out, err := c.UploadPartCopy(ctx, &s3.UploadPartCopyInput{
			Bucket: in.Bucket, Key: in.Key, UploadId: uploadID, PartNumber: aws.Int32(number),
			CopySource: in.CopySource, CopySourceRange: aws.String(fmt.Sprintf("bytes=%d-%d", start, end)),
		})
		if err != nil {
			abort()
			return err
		}
		parts = append(parts, types.CompletedPart{ETag: out.CopyPartResult.ETag, PartNumber: aws.Int32(number)})
	}
	_, err = c.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket: in.Bucket, Key: in.Key, UploadId: uploadID,
		MultipartUpload: &types.CompletedMultipartUpload{Parts: parts},
	})
	if err != nil {
		abort()
	}
	return err
}
