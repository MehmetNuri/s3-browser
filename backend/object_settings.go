package main

import (
	"context"
	"errors"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// ObjectSettings is what the object settings dialog edits beyond the HTTP
// headers: storage class, tags, user metadata, public read access and the
// restore state of archived objects.
type ObjectSettings struct {
	StorageClass string            `json:"storageClass"`
	Tags         map[string]string `json:"tags"`
	Metadata     map[string]string `json:"metadata"`
	Public       bool              `json:"public"`
	Restore      string            `json:"restore"` // the x-amz-restore header, "" when not archived
	Encryption   string            `json:"encryption"`
	Retention    ObjectRetention   `json:"retention"`
	// Settings the provider could not report, with the reason.
	Unsupported map[string]string `json:"unsupported"`
}

// Storage classes a user can pick; providers ignore or reject the ones they lack.
var storageClasses = map[string]bool{
	"STANDARD": true, "REDUCED_REDUNDANCY": true, "STANDARD_IA": true, "ONEZONE_IA": true,
	"INTELLIGENT_TIERING": true, "GLACIER_IR": true, "GLACIER": true, "DEEP_ARCHIVE": true,
}

const (
	maxMetadataCount = 50
	maxRestoreDays   = 365
	// Objects per storage class change; larger selections are refused.
	maxStorageClassKeys = 5000
)

func (a *App) GetObjectSettings(bucket, key string) (ObjectSettings, error) {
	c, err := a.cli()
	if err != nil {
		return ObjectSettings{}, err
	}
	head, err := c.HeadObject(a.ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return ObjectSettings{}, describeErr(err)
	}
	s := ObjectSettings{
		StorageClass: string(head.StorageClass), Metadata: head.Metadata, Tags: map[string]string{},
		Restore: aws.ToString(head.Restore), Encryption: string(head.ServerSideEncryption), Unsupported: map[string]string{},
	}
	if s.StorageClass == "" {
		s.StorageClass = "STANDARD"
	}
	if s.Metadata == nil {
		s.Metadata = map[string]string{}
	}
	if out, err := c.GetObjectTagging(a.ctx, &s3.GetObjectTaggingInput{Bucket: aws.String(bucket), Key: aws.String(key)}); err == nil {
		for _, tag := range out.TagSet {
			s.Tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
		}
	} else if !optionalConfig(err) {
		s.Unsupported["tags"] = describeErr(err).Error()
	}
	if head.ObjectLockMode != "" || head.ObjectLockLegalHoldStatus != "" {
		// The HEAD already told us the object is locked; the details need their own calls.
		retention, unsupported := a.objectRetention(c, bucket, key)
		s.Retention = retention
		for name, reason := range unsupported {
			s.Unsupported[name] = reason
		}
	} else if a.lockAware(c, bucket) {
		retention, unsupported := a.objectRetention(c, bucket, key)
		s.Retention = retention
		for name, reason := range unsupported {
			s.Unsupported[name] = reason
		}
	} else {
		s.Unsupported["retention"] = T("objectLockOff")
	}
	if out, err := c.GetObjectAcl(a.ctx, &s3.GetObjectAclInput{Bucket: aws.String(bucket), Key: aws.String(key)}); err == nil {
		s.Public = grantsPublicRead(out.Grants)
	} else {
		s.Unsupported["acl"] = describeErr(err).Error()
	}
	return s, nil
}

// SetObjectTags replaces the tags of an object; no tags removes them.
func (a *App) SetObjectTags(bucket, key string, tags map[string]string) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	set, err := tagSet(tags)
	if err != nil {
		return err
	}
	if len(set) == 0 {
		_, err = c.DeleteObjectTagging(a.ctx, &s3.DeleteObjectTaggingInput{Bucket: aws.String(bucket), Key: aws.String(key)})
		return describeErr(err)
	}
	_, err = c.PutObjectTagging(a.ctx, &s3.PutObjectTaggingInput{
		Bucket: aws.String(bucket), Key: aws.String(key), Tagging: &types.Tagging{TagSet: set},
	})
	return describeErr(err)
}

// copyInPlace rewrites an object onto itself, which is how S3 changes the
// metadata or storage class of a stored object. The HTTP headers are kept.
func (a *App) copyInPlace(c *s3.Client, bucket, key string, change func(head *s3.HeadObjectOutput, in *s3.CopyObjectInput)) error {
	return a.copyInPlaceCtx(a.ctx, c, bucket, key, change)
}

func (a *App) copyInPlaceCtx(ctx context.Context, c *s3.Client, bucket, key string, change func(head *s3.HeadObjectOutput, in *s3.CopyObjectInput)) error {
	head, err := c.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return describeErr(err)
	}
	// A copy would otherwise fall back to the bucket's default encryption and a private ACL.
	public := false
	if acl, err := c.GetObjectAcl(ctx, &s3.GetObjectAclInput{Bucket: aws.String(bucket), Key: aws.String(key)}); err == nil {
		public = grantsPublicRead(acl.Grants)
	}
	in := &s3.CopyObjectInput{
		Bucket: aws.String(bucket), Key: aws.String(key), CopySource: aws.String(copySource(bucket, key)),
		MetadataDirective: types.MetadataDirectiveReplace, Metadata: head.Metadata, StorageClass: head.StorageClass,
		ContentType: head.ContentType, CacheControl: head.CacheControl,
		ContentDisposition: head.ContentDisposition, ContentEncoding: head.ContentEncoding,
		ServerSideEncryption: head.ServerSideEncryption, SSEKMSKeyId: head.SSEKMSKeyId, BucketKeyEnabled: head.BucketKeyEnabled,
	}
	change(head, in)
	if err := serverCopy(ctx, c, in, aws.ToInt64(head.ContentLength)); err != nil {
		return describeErr(err)
	}
	if public {
		_, err = c.PutObjectAcl(ctx, &s3.PutObjectAclInput{Bucket: aws.String(bucket), Key: aws.String(key), ACL: types.ObjectCannedACLPublicRead})
	}
	return describeErr(err)
}

// grantsPublicRead reports whether everyone may read the object.
func grantsPublicRead(grants []types.Grant) bool {
	for _, grant := range grants {
		if grant.Grantee != nil && strings.HasSuffix(aws.ToString(grant.Grantee.URI), "/global/AllUsers") &&
			(grant.Permission == types.PermissionRead || grant.Permission == types.PermissionFullControl) {
			return true
		}
	}
	return false
}

// SetObjectMetadata replaces the user metadata (x-amz-meta-*) of an object.
func (a *App) SetObjectMetadata(bucket, key string, metadata map[string]string) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	if len(metadata) > maxMetadataCount {
		return errors.New(T("tooManyMetadata", maxMetadataCount))
	}
	clean := make(map[string]string, len(metadata))
	for name, value := range metadata {
		name, value = strings.ToLower(strings.TrimSpace(name)), strings.TrimSpace(value)
		if name == "" || !validHeaderValue(name) || !validHeaderValue(value) ||
			strings.ContainsFunc(name, func(r rune) bool { return r == ' ' || r == ':' || r > 0x7e }) {
			return errors.New(T("invalidMetadata"))
		}
		clean[name] = value
	}
	return a.copyInPlace(c, bucket, key, func(_ *s3.HeadObjectOutput, in *s3.CopyObjectInput) { in.Metadata = clean })
}

// SetStorageClass moves objects to another storage class by copying each one
// onto itself. Folders are not expanded; the frontend passes object keys.
func (a *App) SetStorageClass(bucket string, keys []string, class string) (int, error) {
	c, err := a.cli()
	if err != nil {
		return 0, err
	}
	if !storageClasses[class] {
		return 0, errors.New(T("invalidStorageClass"))
	}
	if len(keys) > maxStorageClassKeys {
		return 0, errors.New(T("tooManyKeys", maxStorageClassKeys))
	}
	// Each object is a queue item, so the change shows progress and can be cancelled.
	var tasks []*transferTask
	for _, key := range keys {
		if strings.HasSuffix(key, "/") {
			continue
		}
		tasks = append(tasks, a.enqueueTransfer("copy", key, 0, func(ctx context.Context, _ func(int64, int64)) error {
			return a.copyInPlaceCtx(ctx, c, bucket, key, func(_ *s3.HeadObjectOutput, in *s3.CopyObjectInput) {
				in.StorageClass = types.StorageClass(class)
			})
		}))
	}
	var first error
	changed, failed := 0, 0
	for _, task := range tasks {
		if err := task.wait(); err != nil {
			failed++
			if first == nil {
				first = err
			}
			continue
		}
		changed++
	}
	if first != nil {
		if failed == 1 && len(keys) == 1 {
			return changed, first
		}
		return changed, errors.New(T("storageClassFailed", failed) + ": " + first.Error())
	}
	return changed, nil
}

// SetObjectPublic grants or revokes read access for everyone through the ACL.
func (a *App) SetObjectPublic(bucket, key string, public bool) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	acl := types.ObjectCannedACLPrivate
	if public {
		acl = types.ObjectCannedACLPublicRead
	}
	_, err = c.PutObjectAcl(a.ctx, &s3.PutObjectAclInput{Bucket: aws.String(bucket), Key: aws.String(key), ACL: acl})
	return describeErr(err)
}

// RestoreObject asks for a temporary copy of an archived (Glacier) object.
func (a *App) RestoreObject(bucket, key string, days int, tier string) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	if days < 1 || days > maxRestoreDays {
		return errors.New(T("invalidRestoreDays", maxRestoreDays))
	}
	request := &types.RestoreRequest{Days: aws.Int32(int32(days))}
	switch tier {
	case "", "Standard":
	case "Bulk", "Expedited":
		request.GlacierJobParameters = &types.GlacierJobParameters{Tier: types.Tier(tier)}
	default:
		return errors.New(T("invalidRestoreTier"))
	}
	_, err = c.RestoreObject(a.ctx, &s3.RestoreObjectInput{Bucket: aws.String(bucket), Key: aws.String(key), RestoreRequest: request})
	return describeErr(err)
}

// lockAware reports whether a bucket has Object Lock enabled, so the object
// dialog shows retention controls only where they can work.
func (a *App) lockAware(c *s3.Client, bucket string) bool {
	lock, err := a.bucketObjectLock(c, bucket)
	return err == nil && lock != nil && lock.Enabled
}
