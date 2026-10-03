package main

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// PublicAccessBlock mirrors the four switches of S3's public access block.
type PublicAccessBlock struct {
	BlockPublicAcls       bool `json:"blockPublicAcls"`
	IgnorePublicAcls      bool `json:"ignorePublicAcls"`
	BlockPublicPolicy     bool `json:"blockPublicPolicy"`
	RestrictPublicBuckets bool `json:"restrictPublicBuckets"`
}

// BucketSettings collects the bucket configuration that the settings dialog
// edits. Rules are handed to the frontend as JSON text, so they can be edited
// in full instead of through a form that covers only part of the schema.
type BucketSettings struct {
	Region            string             `json:"region"`
	Versioning        string             `json:"versioning"` // Enabled | Suspended | ""
	Policy            string             `json:"policy"`     // JSON document or ""
	Cors              string             `json:"cors"`       // JSON array of CORS rules or ""
	Lifecycle         string             `json:"lifecycle"`  // JSON array of lifecycle rules or ""
	Tags              map[string]string  `json:"tags"`
	Encryption        string             `json:"encryption"` // "" | AES256 | aws:kms
	KMSKey            string             `json:"kmsKey"`
	PublicAccessBlock *PublicAccessBlock `json:"publicAccessBlock"` // nil when the provider has none
	// Static website hosting: index document ("" when hosting is off) and error document.
	WebsiteIndex string `json:"websiteIndex"`
	WebsiteError string `json:"websiteError"`
	// Access logging target bucket ("" when off) and key prefix.
	LoggingBucket string      `json:"loggingBucket"`
	LoggingPrefix string      `json:"loggingPrefix"`
	RequesterPays bool        `json:"requesterPays"`
	ObjectLock    *ObjectLock `json:"objectLock"` // nil when the provider has none
	// Settings the provider could not report, with the reason.
	Unsupported map[string]string `json:"unsupported"`
}

const (
	maxPolicySize = 20 * 1024
	maxRulesSize  = 64 * 1024
	maxTagCount   = 50
)

// settingErr explains a failed configuration write. S3-compatible services
// that lack a sub-resource often route the PUT to "create bucket" and answer
// BucketAlreadyExists, which is meaningless to the user.
func settingErr(err error) error {
	var ae smithy.APIError
	if errors.As(err, &ae) && (ae.ErrorCode() == "BucketAlreadyExists" || ae.ErrorCode() == "BucketAlreadyOwnedByYou") {
		return errors.New(T("settingUnsupported"))
	}
	return describeErr(err)
}

// optionalConfig reports whether an error only says that a configuration
// was never set, which is a normal state and not a failure.
func optionalConfig(err error) bool {
	var ae smithy.APIError
	return errors.As(err, &ae) && configMissing[ae.ErrorCode()]
}

// GetBucketSettings reads every supported configuration of a bucket. A
// provider that lacks one of them is reported in Unsupported, so the dialog
// still opens for MinIO, R2 or Supabase.
func (a *App) GetBucketSettings(bucket string) (BucketSettings, error) {
	c, err := a.cli()
	if err != nil {
		return BucketSettings{}, err
	}
	if bucket == "" {
		return BucketSettings{}, errors.New(T("bucketRequired"))
	}
	s := BucketSettings{Tags: map[string]string{}, Unsupported: map[string]string{}}
	note := func(name string, err error) {
		if err != nil && !optionalConfig(err) {
			s.Unsupported[name] = describeErr(err).Error()
		}
	}
	if out, err := c.GetBucketLocation(a.ctx, &s3.GetBucketLocationInput{Bucket: aws.String(bucket)}); err == nil {
		s.Region = string(out.LocationConstraint)
		if s.Region == "" {
			s.Region = "us-east-1"
		}
	}
	if out, err := c.GetBucketVersioning(a.ctx, &s3.GetBucketVersioningInput{Bucket: aws.String(bucket)}); err == nil {
		s.Versioning = string(out.Status)
	} else {
		note("versioning", err)
	}
	if out, err := c.GetBucketPolicy(a.ctx, &s3.GetBucketPolicyInput{Bucket: aws.String(bucket)}); err == nil {
		// Some providers answer the policy request with something else, like a listing.
		if policy := aws.ToString(out.Policy); json.Valid([]byte(policy)) {
			s.Policy = prettyJSON(policy)
		} else if strings.TrimSpace(policy) != "" {
			s.Unsupported["policy"] = T("policyNotJSON")
		}
	} else {
		note("policy", err)
	}
	if out, err := c.GetBucketCors(a.ctx, &s3.GetBucketCorsInput{Bucket: aws.String(bucket)}); err == nil {
		s.Cors = rulesJSON(out.CORSRules)
	} else {
		note("cors", err)
	}
	if out, err := c.GetBucketLifecycleConfiguration(a.ctx, &s3.GetBucketLifecycleConfigurationInput{Bucket: aws.String(bucket)}); err == nil {
		s.Lifecycle = rulesJSON(out.Rules)
	} else {
		note("lifecycle", err)
	}
	if out, err := c.GetBucketTagging(a.ctx, &s3.GetBucketTaggingInput{Bucket: aws.String(bucket)}); err == nil {
		for _, tag := range out.TagSet {
			s.Tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
		}
	} else {
		note("tags", err)
	}
	if out, err := c.GetBucketEncryption(a.ctx, &s3.GetBucketEncryptionInput{Bucket: aws.String(bucket)}); err == nil {
		if cfg := out.ServerSideEncryptionConfiguration; cfg != nil && len(cfg.Rules) > 0 && cfg.Rules[0].ApplyServerSideEncryptionByDefault != nil {
			def := cfg.Rules[0].ApplyServerSideEncryptionByDefault
			s.Encryption, s.KMSKey = string(def.SSEAlgorithm), aws.ToString(def.KMSMasterKeyID)
		}
	} else {
		note("encryption", err)
	}
	if out, err := c.GetBucketWebsite(a.ctx, &s3.GetBucketWebsiteInput{Bucket: aws.String(bucket)}); err == nil {
		if out.IndexDocument != nil {
			s.WebsiteIndex = aws.ToString(out.IndexDocument.Suffix)
		}
		if out.ErrorDocument != nil {
			s.WebsiteError = aws.ToString(out.ErrorDocument.Key)
		}
	} else {
		note("website", err)
	}
	if out, err := c.GetBucketLogging(a.ctx, &s3.GetBucketLoggingInput{Bucket: aws.String(bucket)}); err == nil {
		if out.LoggingEnabled != nil {
			s.LoggingBucket, s.LoggingPrefix = aws.ToString(out.LoggingEnabled.TargetBucket), aws.ToString(out.LoggingEnabled.TargetPrefix)
		}
	} else {
		note("logging", err)
	}
	if out, err := c.GetBucketRequestPayment(a.ctx, &s3.GetBucketRequestPaymentInput{Bucket: aws.String(bucket)}); err == nil {
		s.RequesterPays = out.Payer == types.PayerRequester
	} else {
		note("requesterPays", err)
	}
	if lock, err := a.bucketObjectLock(c, bucket); err == nil {
		s.ObjectLock = lock
	} else {
		note("objectLock", err)
	}
	if out, err := c.GetPublicAccessBlock(a.ctx, &s3.GetPublicAccessBlockInput{Bucket: aws.String(bucket)}); err == nil {
		if cfg := out.PublicAccessBlockConfiguration; cfg != nil {
			s.PublicAccessBlock = &PublicAccessBlock{
				BlockPublicAcls: aws.ToBool(cfg.BlockPublicAcls), IgnorePublicAcls: aws.ToBool(cfg.IgnorePublicAcls),
				BlockPublicPolicy: aws.ToBool(cfg.BlockPublicPolicy), RestrictPublicBuckets: aws.ToBool(cfg.RestrictPublicBuckets),
			}
		}
	} else if optionalConfig(err) {
		s.PublicAccessBlock = &PublicAccessBlock{}
	} else {
		note("publicAccessBlock", err)
	}
	return s, nil
}

func prettyJSON(text string) string {
	var v any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		return text
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return text
	}
	return string(b)
}

// rulesJSON renders SDK rule structs as editable JSON; an empty list is "".
func rulesJSON[R any](rules []R) string {
	if len(rules) == 0 {
		return ""
	}
	b, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}

// parseRules reads the JSON the dialog sends back into SDK rule structs.
func parseRules[R any](text string, limit int) ([]R, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	if len(text) > limit {
		return nil, errors.New(T("rulesTooLarge"))
	}
	var rules []R
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&rules); err != nil {
		return nil, errors.New(T("invalidRules", err.Error()))
	}
	return rules, nil
}

// SetBucketPolicy stores a policy document; an empty document removes it.
func (a *App) SetBucketPolicy(bucket, policy string) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	policy = strings.TrimSpace(policy)
	if policy == "" {
		_, err = c.DeleteBucketPolicy(a.ctx, &s3.DeleteBucketPolicyInput{Bucket: aws.String(bucket)})
		return describeErr(err)
	}
	if len(policy) > maxPolicySize || !json.Valid([]byte(policy)) {
		return errors.New(T("invalidPolicy"))
	}
	_, err = c.PutBucketPolicy(a.ctx, &s3.PutBucketPolicyInput{Bucket: aws.String(bucket), Policy: aws.String(policy)})
	return settingErr(err)
}

// SetBucketCors stores CORS rules given as JSON; an empty list removes them.
func (a *App) SetBucketCors(bucket, rules string) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	parsed, err := parseRules[types.CORSRule](rules, maxRulesSize)
	if err != nil {
		return err
	}
	if len(parsed) == 0 {
		_, err = c.DeleteBucketCors(a.ctx, &s3.DeleteBucketCorsInput{Bucket: aws.String(bucket)})
		return describeErr(err)
	}
	_, err = c.PutBucketCors(a.ctx, &s3.PutBucketCorsInput{
		Bucket: aws.String(bucket), CORSConfiguration: &types.CORSConfiguration{CORSRules: parsed},
	})
	return settingErr(err)
}

// SetBucketLifecycle stores lifecycle rules given as JSON; an empty list removes them.
func (a *App) SetBucketLifecycle(bucket, rules string) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	parsed, err := parseRules[types.LifecycleRule](rules, maxRulesSize)
	if err != nil {
		return err
	}
	if len(parsed) == 0 {
		_, err = c.DeleteBucketLifecycle(a.ctx, &s3.DeleteBucketLifecycleInput{Bucket: aws.String(bucket)})
		return describeErr(err)
	}
	_, err = c.PutBucketLifecycleConfiguration(a.ctx, &s3.PutBucketLifecycleConfigurationInput{
		Bucket: aws.String(bucket), LifecycleConfiguration: &types.BucketLifecycleConfiguration{Rules: parsed},
	})
	return settingErr(err)
}

func tagSet(tags map[string]string) ([]types.Tag, error) {
	if len(tags) > maxTagCount {
		return nil, errors.New(T("tooManyTags", maxTagCount))
	}
	keys := make([]string, 0, len(tags))
	for key := range tags {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	set := make([]types.Tag, 0, len(keys))
	for _, key := range keys {
		key, value := strings.TrimSpace(key), strings.TrimSpace(tags[key])
		if key == "" || len(key) > 128 || len(value) > 256 || !validHeaderValue(key) || !validHeaderValue(value) {
			return nil, errors.New(T("invalidTag"))
		}
		set = append(set, types.Tag{Key: aws.String(key), Value: aws.String(value)})
	}
	return set, nil
}

// SetBucketTags replaces the tags of a bucket; no tags removes the tag set.
func (a *App) SetBucketTags(bucket string, tags map[string]string) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	set, err := tagSet(tags)
	if err != nil {
		return err
	}
	if len(set) == 0 {
		_, err = c.DeleteBucketTagging(a.ctx, &s3.DeleteBucketTaggingInput{Bucket: aws.String(bucket)})
		return describeErr(err)
	}
	_, err = c.PutBucketTagging(a.ctx, &s3.PutBucketTaggingInput{Bucket: aws.String(bucket), Tagging: &types.Tagging{TagSet: set}})
	return settingErr(err)
}

// SetBucketEncryption sets the default encryption: "" (none), AES256 or aws:kms
// with an optional key.
func (a *App) SetBucketEncryption(bucket, algorithm, kmsKey string) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	kmsKey = strings.TrimSpace(kmsKey)
	switch algorithm {
	case "":
		_, err = c.DeleteBucketEncryption(a.ctx, &s3.DeleteBucketEncryptionInput{Bucket: aws.String(bucket)})
		return describeErr(err)
	case string(types.ServerSideEncryptionAes256), string(types.ServerSideEncryptionAwsKms):
	default:
		return errors.New(T("invalidEncryption"))
	}
	def := &types.ServerSideEncryptionByDefault{SSEAlgorithm: types.ServerSideEncryption(algorithm)}
	if algorithm == string(types.ServerSideEncryptionAwsKms) && kmsKey != "" {
		if !validHeaderValue(kmsKey) {
			return errors.New(T("invalidEncryption"))
		}
		def.KMSMasterKeyID = aws.String(kmsKey)
	}
	_, err = c.PutBucketEncryption(a.ctx, &s3.PutBucketEncryptionInput{
		Bucket: aws.String(bucket),
		ServerSideEncryptionConfiguration: &types.ServerSideEncryptionConfiguration{
			Rules: []types.ServerSideEncryptionRule{{ApplyServerSideEncryptionByDefault: def}},
		},
	})
	return settingErr(err)
}

// SetPublicAccessBlock stores the four public access switches.
func (a *App) SetPublicAccessBlock(bucket string, block PublicAccessBlock) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	_, err = c.PutPublicAccessBlock(a.ctx, &s3.PutPublicAccessBlockInput{
		Bucket: aws.String(bucket),
		PublicAccessBlockConfiguration: &types.PublicAccessBlockConfiguration{
			BlockPublicAcls: aws.Bool(block.BlockPublicAcls), IgnorePublicAcls: aws.Bool(block.IgnorePublicAcls),
			BlockPublicPolicy: aws.Bool(block.BlockPublicPolicy), RestrictPublicBuckets: aws.Bool(block.RestrictPublicBuckets),
		},
	})
	return settingErr(err)
}

// SetBucketWebsite turns static website hosting on with the given documents,
// or off when the index document is empty.
func (a *App) SetBucketWebsite(bucket, index, errorDocument string) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	index, errorDocument = strings.TrimSpace(index), strings.TrimSpace(errorDocument)
	if index == "" {
		_, err = c.DeleteBucketWebsite(a.ctx, &s3.DeleteBucketWebsiteInput{Bucket: aws.String(bucket)})
		return describeErr(err)
	}
	if !validHeaderValue(index) || !validHeaderValue(errorDocument) || strings.Contains(index, "/") {
		return errors.New(T("invalidWebsite"))
	}
	cfg := &types.WebsiteConfiguration{IndexDocument: &types.IndexDocument{Suffix: aws.String(index)}}
	if errorDocument != "" {
		cfg.ErrorDocument = &types.ErrorDocument{Key: aws.String(errorDocument)}
	}
	_, err = c.PutBucketWebsite(a.ctx, &s3.PutBucketWebsiteInput{Bucket: aws.String(bucket), WebsiteConfiguration: cfg})
	return settingErr(err)
}

// SetBucketLogging sends access logs to another bucket, or stops logging when
// the target is empty.
func (a *App) SetBucketLogging(bucket, target, prefix string) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	target, prefix = strings.TrimSpace(target), strings.TrimSpace(prefix)
	status := &types.BucketLoggingStatus{}
	if target != "" {
		if !validHeaderValue(target) || !validHeaderValue(prefix) {
			return errors.New(T("invalidLogging"))
		}
		status.LoggingEnabled = &types.LoggingEnabled{TargetBucket: aws.String(target), TargetPrefix: aws.String(prefix)}
	}
	_, err = c.PutBucketLogging(a.ctx, &s3.PutBucketLoggingInput{Bucket: aws.String(bucket), BucketLoggingStatus: status})
	return settingErr(err)
}

// SetRequesterPays makes the requester, instead of the bucket owner, pay for requests.
func (a *App) SetRequesterPays(bucket string, enabled bool) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	payer := types.PayerBucketOwner
	if enabled {
		payer = types.PayerRequester
	}
	_, err = c.PutBucketRequestPayment(a.ctx, &s3.PutBucketRequestPaymentInput{
		Bucket: aws.String(bucket), RequestPaymentConfiguration: &types.RequestPaymentConfiguration{Payer: payer},
	})
	return settingErr(err)
}
