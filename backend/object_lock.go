package main

import (
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// ObjectLock describes a bucket's Object Lock state and default retention.
type ObjectLock struct {
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode"` // GOVERNANCE | COMPLIANCE | ""
	Days    int32  `json:"days"`
	Years   int32  `json:"years"`
}

// ObjectRetention is the lock state of one object.
type ObjectRetention struct {
	Mode      string `json:"mode"` // GOVERNANCE | COMPLIANCE | ""
	Until     string `json:"until"`
	LegalHold bool   `json:"legalHold"`
}

const maxRetentionYears = 100

func (a *App) bucketObjectLock(c *s3.Client, bucket string) (*ObjectLock, error) {
	out, err := c.GetObjectLockConfiguration(a.ctx, &s3.GetObjectLockConfigurationInput{Bucket: aws.String(bucket)})
	if err != nil {
		if optionalConfig(err) {
			return &ObjectLock{}, nil
		}
		return nil, err
	}
	lock := &ObjectLock{}
	if cfg := out.ObjectLockConfiguration; cfg != nil {
		lock.Enabled = cfg.ObjectLockEnabled == types.ObjectLockEnabledEnabled
		if cfg.Rule != nil && cfg.Rule.DefaultRetention != nil {
			lock.Mode = string(cfg.Rule.DefaultRetention.Mode)
			lock.Days, lock.Years = aws.ToInt32(cfg.Rule.DefaultRetention.Days), aws.ToInt32(cfg.Rule.DefaultRetention.Years)
		}
	}
	return lock, nil
}

func retentionMode(mode string) (types.ObjectLockRetentionMode, error) {
	switch mode {
	case string(types.ObjectLockRetentionModeGovernance), string(types.ObjectLockRetentionModeCompliance):
		return types.ObjectLockRetentionMode(mode), nil
	}
	return "", errors.New(T("invalidRetention"))
}

// SetObjectLock enables Object Lock on a bucket, with an optional default
// retention. Object Lock cannot be turned off again; an empty mode only
// removes the default retention.
func (a *App) SetObjectLock(bucket, mode string, days, years int32) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	cfg := &types.ObjectLockConfiguration{ObjectLockEnabled: types.ObjectLockEnabledEnabled}
	if mode != "" {
		m, err := retentionMode(mode)
		if err != nil {
			return err
		}
		if (days <= 0) == (years <= 0) || days > 36500 || years > maxRetentionYears {
			return errors.New(T("invalidRetentionPeriod"))
		}
		retention := &types.DefaultRetention{Mode: m}
		if days > 0 {
			retention.Days = aws.Int32(days)
		} else {
			retention.Years = aws.Int32(years)
		}
		cfg.Rule = &types.ObjectLockRule{DefaultRetention: retention}
	}
	_, err = c.PutObjectLockConfiguration(a.ctx, &s3.PutObjectLockConfigurationInput{Bucket: aws.String(bucket), ObjectLockConfiguration: cfg})
	return settingErr(err)
}

func (a *App) objectRetention(c *s3.Client, bucket, key string) (ObjectRetention, map[string]string) {
	r, unsupported := ObjectRetention{}, map[string]string{}
	if out, err := c.GetObjectRetention(a.ctx, &s3.GetObjectRetentionInput{Bucket: aws.String(bucket), Key: aws.String(key)}); err == nil {
		if out.Retention != nil {
			r.Mode = string(out.Retention.Mode)
			if out.Retention.RetainUntilDate != nil {
				r.Until = out.Retention.RetainUntilDate.UTC().Format(time.RFC3339)
			}
		}
	} else if !optionalConfig(err) && !noLockConfiguration(err) {
		unsupported["retention"] = describeErr(err).Error()
	}
	if out, err := c.GetObjectLegalHold(a.ctx, &s3.GetObjectLegalHoldInput{Bucket: aws.String(bucket), Key: aws.String(key)}); err == nil {
		r.LegalHold = out.LegalHold != nil && out.LegalHold.Status == types.ObjectLockLegalHoldStatusOn
	} else if !optionalConfig(err) && !noLockConfiguration(err) {
		unsupported["legalHold"] = describeErr(err).Error()
	}
	return r, unsupported
}

// noLockConfiguration reports the errors S3 gives for an object without a
// retention or legal hold, and for buckets without Object Lock.
func noLockConfiguration(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "nosuchobjectlockconfiguration") || strings.Contains(msg, "invalidrequest") && strings.Contains(msg, "object lock")
}

// SetObjectRetention sets or extends the retention of an object. Shortening a
// GOVERNANCE retention needs the bypass flag; COMPLIANCE cannot be shortened.
func (a *App) SetObjectRetention(bucket, key, mode, until string, bypassGovernance bool) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	m, err := retentionMode(mode)
	if err != nil {
		return err
	}
	date, err := time.Parse(time.RFC3339, strings.TrimSpace(until))
	if err != nil {
		date, err = time.Parse("2006-01-02", strings.TrimSpace(until))
	}
	if err != nil || date.Before(time.Now()) {
		return errors.New(T("invalidRetentionDate"))
	}
	in := &s3.PutObjectRetentionInput{
		Bucket: aws.String(bucket), Key: aws.String(key),
		Retention: &types.ObjectLockRetention{Mode: m, RetainUntilDate: aws.Time(date)},
	}
	if bypassGovernance {
		in.BypassGovernanceRetention = aws.Bool(true)
	}
	_, err = c.PutObjectRetention(a.ctx, in)
	return describeErr(err)
}

// SetObjectLegalHold places or lifts a legal hold on an object.
func (a *App) SetObjectLegalHold(bucket, key string, on bool) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	status := types.ObjectLockLegalHoldStatusOff
	if on {
		status = types.ObjectLockLegalHoldStatusOn
	}
	_, err = c.PutObjectLegalHold(a.ctx, &s3.PutObjectLegalHoldInput{
		Bucket: aws.String(bucket), Key: aws.String(key), LegalHold: &types.ObjectLockLegalHold{Status: status},
	})
	return describeErr(err)
}
