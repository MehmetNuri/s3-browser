package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
)

// Distribution is a CloudFront distribution that serves the bucket.
type Distribution struct {
	ID      string   `json:"id"`
	Domain  string   `json:"domain"`
	Status  string   `json:"status"`
	Enabled bool     `json:"enabled"`
	Comment string   `json:"comment"`
	Aliases []string `json:"aliases"`
	Origin  string   `json:"origin"`
}

const maxInvalidationPaths = 100

// cloudFrontClient builds a client from the active profile; only Amazon S3
// profiles can have distributions.
func (a *App) cloudFrontClient() (*cloudfront.Client, error) {
	a.mu.RLock()
	p := a.profile
	a.mu.RUnlock()
	if p.Provider != "aws" {
		return nil, errors.New(T("cloudFrontAwsOnly"))
	}
	if isSealed(p.SecretKey) || isSealed(p.SessionToken) {
		return nil, errors.New(T("secretsLocked"))
	}
	cfg, err := config.LoadDefaultConfig(a.ctx,
		config.WithRegion("us-east-1"), // CloudFront is a global service
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(p.AccessKey, p.SecretKey, p.SessionToken)),
	)
	if err != nil {
		return nil, err
	}
	return cloudfront.NewFromConfig(cfg), nil
}

// ListDistributions returns the distributions whose origin is the bucket.
func (a *App) ListDistributions(bucket string) ([]Distribution, error) {
	c, err := a.cloudFrontClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	var found []Distribution
	p := cloudfront.NewListDistributionsPaginator(c, &cloudfront.ListDistributionsInput{})
	for p.HasMorePages() {
		out, err := p.NextPage(ctx)
		if err != nil {
			return nil, describeErr(err)
		}
		if out.DistributionList == nil {
			break
		}
		for _, d := range out.DistributionList.Items {
			origin := bucketOrigin(d.Origins, bucket)
			if origin == "" {
				continue
			}
			dist := Distribution{
				ID: aws.ToString(d.Id), Domain: aws.ToString(d.DomainName), Status: aws.ToString(d.Status),
				Enabled: aws.ToBool(d.Enabled), Comment: aws.ToString(d.Comment), Origin: origin, Aliases: []string{},
			}
			if d.Aliases != nil {
				dist.Aliases = d.Aliases.Items
			}
			found = append(found, dist)
		}
	}
	if found == nil {
		found = []Distribution{}
	}
	return found, nil
}

// bucketOrigin returns the origin domain of a distribution that points at the
// bucket, through either the REST or the website endpoint.
func bucketOrigin(origins *types.Origins, bucket string) string {
	if origins == nil {
		return ""
	}
	for _, o := range origins.Items {
		domain := aws.ToString(o.DomainName)
		if strings.HasPrefix(domain, bucket+".s3") || strings.HasPrefix(domain, bucket+".s3-website") {
			return domain
		}
	}
	return ""
}

// InvalidatePaths asks CloudFront to drop cached copies of the given paths,
// such as "/index.html" or "/images/*".
func (a *App) InvalidatePaths(distributionID string, paths []string) (string, error) {
	c, err := a.cloudFrontClient()
	if err != nil {
		return "", err
	}
	var clean []string
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		if !validHeaderValue(path) {
			return "", errors.New(T("invalidInvalidationPath"))
		}
		clean = append(clean, path)
	}
	if len(clean) == 0 || len(clean) > maxInvalidationPaths || !validHeaderValue(distributionID) || distributionID == "" {
		return "", errors.New(T("invalidInvalidationPath"))
	}
	out, err := c.CreateInvalidation(a.ctx, &cloudfront.CreateInvalidationInput{
		DistributionId: aws.String(distributionID),
		InvalidationBatch: &types.InvalidationBatch{
			CallerReference: aws.String(fmt.Sprintf("s3browser-%d", time.Now().UnixNano())),
			Paths:           &types.Paths{Quantity: aws.Int32(int32(len(clean))), Items: clean},
		},
	})
	if err != nil {
		return "", describeErr(err)
	}
	if out.Invalidation == nil {
		return "", nil
	}
	return aws.ToString(out.Invalidation.Id), nil
}
