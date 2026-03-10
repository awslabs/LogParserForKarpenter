// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
package s3

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	lp4k "github.com/awslabs/LogParserForKarpenter/parser"
)

const (
	// environment variables
	s3BucketEnv = "LP4K_S3_BUCKET"
	s3PrefixEnv = "LP4K_S3_PREFIX"
	s3RegionEnv = "LP4K_S3_REGION"
)

var s3Bucket, s3Prefix, s3Region string
var s3Enabled bool

// Initialize S3 configuration from environment variables
func init() {
	s3Bucket = os.Getenv(s3BucketEnv)
	s3Prefix = os.Getenv(s3PrefixEnv)
	s3Region = os.Getenv(s3RegionEnv)

	// S3 is enabled only if bucket is specified
	s3Enabled = s3Bucket != ""

	if s3Enabled {
		if s3Prefix == "" {
			s3Prefix = "karpenter-logs"
		}
		if s3Region == "" {
			s3Region = "us-east-1"
		}
		fmt.Fprintf(os.Stderr, "S3 upload enabled: bucket=%s, prefix=%s, region=%s\n", s3Bucket, s3Prefix, s3Region)
	}
}

// IsEnabled returns whether S3 upload is configured
func IsEnabled() bool {
	return s3Enabled
}

// GetConfig returns the S3 configuration
func GetConfig() (bucket, prefix, region string) {
	return s3Bucket, s3Prefix, s3Region
}

// UploadToS3 uploads the nodeclaim CSV data to S3
func UploadToS3(ctx context.Context, nodeclaimmap *map[string]lp4k.Nodeclaimstruct) error {
	if !s3Enabled {
		return nil
	}

	// Load AWS configuration
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(s3Region))
	if err != nil {
		return fmt.Errorf("unable to load AWS SDK config: %w", err)
	}

	// Create S3 client
	client := s3.NewFromConfig(cfg)

	// Convert nodeclaimmap to CSV
	csvData := lp4k.ConvertToCSV(nodeclaimmap)

	// Generate S3 key with timestamp
	timestamp := time.Now().Format("2006-01-02-15-04-05")
	s3Key := fmt.Sprintf("%s/karpenter-nodeclaims-%s.csv", strings.TrimSuffix(s3Prefix, "/"), timestamp)

	// Upload to S3
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s3Bucket),
		Key:         aws.String(s3Key),
		Body:        bytes.NewReader([]byte(csvData)),
		ContentType: aws.String("text/csv"),
	})

	if err != nil {
		return fmt.Errorf("failed to upload to S3: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Successfully uploaded to s3://%s/%s\n", s3Bucket, s3Key)
	return nil
}