package main

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
)

func TestCloudFrontNeedsAmazonProfile(t *testing.T) {
	a := connectedApp(t, gofakes3.New(s3mem.New()).Server())
	if _, err := a.ListDistributions("test"); err == nil || err.Error() != T("cloudFrontAwsOnly") {
		t.Fatalf("minio profile listed distributions: %v", err)
	}
	if _, err := a.InvalidatePaths("E123", []string{"/x"}); err == nil {
		t.Fatal("invalidation on a minio profile was accepted")
	}
	origins := &types.Origins{Items: []types.Origin{
		{DomainName: aws.String("other.s3.amazonaws.com")},
		{DomainName: aws.String("photos.s3-website-eu-west-1.amazonaws.com")},
	}}
	if got := bucketOrigin(origins, "photos"); got != "photos.s3-website-eu-west-1.amazonaws.com" {
		t.Fatalf("origin: %q", got)
	}
	if got := bucketOrigin(origins, "photo"); got != "" {
		t.Fatalf("prefix matched a different bucket: %q", got)
	}
}
