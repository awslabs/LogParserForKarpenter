// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
package s3

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	lp4k "github.com/awslabs/LogParserForKarpenter/parser"
)

const (
	// environment variables
	s3BucketEnv    = "LP4K_S3_BUCKET"
	s3PrefixEnv    = "LP4K_S3_PREFIX"
	s3RegionEnv    = "LP4K_S3_REGION"
	s3OverwriteEnv = "LP4K_S3_OVERWRITE"
	timeFormatEnv  = "LP4K_TIME_FORMAT"
	// context timeouts
	configTimeout = 5 * time.Second
	uploadTimeout = 30 * time.Second
	// default time format
	defaultTimeFormat = "2006-01-02-15-04-05"
)

var s3Bucket, s3Prefix, s3Region string
var s3Enabled, s3Overwrite bool
var s3Client *s3.Client
var once sync.Once
var clientErr error
var startTimestamp string
var timeFormat string

// Initialize S3 configuration from environment variables
func init() {
	timeFormat = getEnvOrDefault(timeFormatEnv, defaultTimeFormat)
	// Validate time format by attempting to parse a formatted time
	testTime := time.Now()
	formatted := testTime.Format(timeFormat)
	if _, err := time.Parse(timeFormat, formatted); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Invalid LP4K_TIME_FORMAT \"%s\", falling back to default \"%s\"\n", timeFormat, defaultTimeFormat)
		timeFormat = defaultTimeFormat
	}
	s3Bucket = os.Getenv(s3BucketEnv)
	s3Prefix = getEnvOrDefault(s3PrefixEnv, "karpenter-logs")
	s3Region = getEnvOrDefault(s3RegionEnv, "us-east-1")
	s3Overwrite = getEnvBoolOrDefault(s3OverwriteEnv, false)
	startTimestamp = time.Now().Format(timeFormat)
	// S3 is enabled only if bucket is specified
	s3Enabled = s3Bucket != ""
	if s3Enabled {
		mode := "timestamped mode"
		if s3Overwrite {
			mode = "overwrite mode"
		}
		fmt.Fprintf(os.Stderr, "S3 upload enabled: bucket=%s, prefix=%s, region=%s (%s)\n", s3Bucket, s3Prefix, s3Region, mode)
	}
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvBoolOrDefault(key string, defaultVal bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	b, _ := strconv.ParseBool(val)
	return b
}

// getS3Client returns a cached S3 client, creating it once on first call
// Uses sync.Once to ensure thread-safe singleton pattern
func getS3Client(ctx context.Context) (*s3.Client, error) {
	once.Do(func() {
		// Create context with timeout for config loading
		cfgCtx, cancel := context.WithTimeout(ctx, configTimeout)
		defer cancel()
		cfg, err := config.LoadDefaultConfig(cfgCtx, config.WithRegion(s3Region))
		if err != nil {
			clientErr = fmt.Errorf("unable to load AWS SDK config: %w", err)
			return
		}
		s3Client = s3.NewFromConfig(cfg)
	})
	return s3Client, clientErr
}

// IsEnabled returns whether S3 upload is configured
func IsEnabled() bool {
	return s3Enabled
}

// GetStartTimestamp returns the program start timestamp in format YYYY-MM-DD-HH-MM-SS
func GetStartTimestamp() string {
	return startTimestamp
}

// UploadToS3 uploads the nodeclaim CSV data to S3 with timeout and context cancellation support
// The S3 client is cached and reused across multiple calls for efficiency
// If LP4K_S3_OVERWRITE=true, the same object is overwritten on each call
// Otherwise, a new timestamped object is created on each call
func UploadToS3(nodeclaimmap *map[string]lp4k.Nodeclaimstruct) error {
	if !s3Enabled {
		return nil
	}
	// Create context with upload timeout
	uploadCtx, cancel := context.WithTimeout(context.Background(), uploadTimeout)
	defer cancel()
	// Get cached S3 client
	client, err := getS3Client(uploadCtx)
	if err != nil {
		return err
	}
	// Convert nodeclaimmap to CSV
	csvData := lp4k.ConvertToCSV(nodeclaimmap)
	// Generate S3 key
	var s3Key string
	if s3Overwrite {
		// Use start timestamp for overwrite mode (same key on each update)
		s3Key = fmt.Sprintf("%s/karpenter-nodeclaims-%s.csv", strings.TrimSuffix(s3Prefix, "/"), startTimestamp)
	} else {
		// Use current timestamp for timestamped mode (new key on each update)
		timestamp := time.Now().Format(timeFormat)
		s3Key = fmt.Sprintf("%s/karpenter-nodeclaims-%s.csv", strings.TrimSuffix(s3Prefix, "/"), timestamp)
	}
	// Upload to S3
	_, err = client.PutObject(uploadCtx, &s3.PutObjectInput{
		Bucket:      aws.String(s3Bucket),
		Key:         aws.String(s3Key),
		Body:        bytes.NewReader([]byte(csvData)),
		ContentType: aws.String("text/csv"),
	})
	// Check context state for better error messages
	if err != nil {
		switch uploadCtx.Err() {
		case context.Canceled:
			return fmt.Errorf("S3 upload cancelled: %w", err)
		case context.DeadlineExceeded:
			return fmt.Errorf("S3 upload timeout exceeded: %w", err)
		default:
			return fmt.Errorf("failed to upload to S3: %w", err)
		}
	}
	fmt.Fprintf(os.Stderr, "Successfully uploaded to s3://%s/%s\n", s3Bucket, s3Key)
	return nil
}
