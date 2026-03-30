package ec2

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/facts/facts-aws-compute/internal/config"
	"github.com/facts/facts-aws-compute/internal/output"
)

// Client wraps the EC2 service client.
type Client struct {
	inner *ec2.Client
}

// New creates a new EC2 client from an aws.Config.
func New(cfg aws.Config) *Client {
	return &Client{inner: ec2.NewFromConfig(cfg)}
}

// InstanceInfo is the canonical output shape for describe-instances.
type InstanceInfo struct {
	InstanceId       string `json:"InstanceId"`
	AmiId            string `json:"AmiId"`
	Region           string `json:"Region"`
	AvailabilityZone string `json:"AvailabilityZone"`
	PrivateIpAddress string `json:"PrivateIpAddress"`
	PublicIpAddress  string `json:"PublicIpAddress,omitempty"`
	Tags             []Tag  `json:"Tags"`
}

// Tag is a simple key-value pair.
type Tag struct {
	Key   string `json:"Key"`
	Value string `json:"Value"`
}

// DescribeInstance calls the EC2 DescribeInstances API for a single instance.
func (c *Client) DescribeInstance(ctx context.Context, instanceID, region string) (*InstanceInfo, error) {
	output.Debugf("EC2 DescribeInstances: %s", instanceID)

	// Avoid unbounded network hangs: apply a default timeout if caller
	// did not provide a deadline on the context.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		t := config.EC2Timeout(10 * time.Second)
		output.Debugf("applying EC2 timeout: %s", t)
		ctx, cancel = context.WithTimeout(ctx, t)
		defer cancel()
	}

	out, err := c.inner.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		return nil, fmt.Errorf("DescribeInstances(%s): %w", instanceID, err)
	}

	for _, res := range out.Reservations {
		for _, inst := range res.Instances {
			info := &InstanceInfo{
				InstanceId: derefStr(inst.InstanceId),
				AmiId:      derefStr(inst.ImageId),
				Region:     region,
			}
			if inst.Placement != nil {
				info.AvailabilityZone = derefStr(inst.Placement.AvailabilityZone)
			}
			info.PrivateIpAddress = derefStr(inst.PrivateIpAddress)
			info.PublicIpAddress = derefStr(inst.PublicIpAddress)
			info.Tags = convertTags(inst.Tags)
			return info, nil
		}
	}

	return nil, fmt.Errorf("DescribeInstances(%s): no instance found in response", instanceID)
}

// DescribeTags calls the EC2 DescribeTags API filtered to a specific instance.
func (c *Client) DescribeTags(ctx context.Context, instanceID string) ([]Tag, error) {
	output.Debugf("EC2 DescribeTags: %s", instanceID)
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		t := config.EC2Timeout(10 * time.Second)
		output.Debugf("applying EC2 timeout: %s", t)
		ctx, cancel = context.WithTimeout(ctx, t)
		defer cancel()
	}

	out, err := c.inner.DescribeTags(ctx, &ec2.DescribeTagsInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("resource-id"),
				Values: []string{instanceID},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("DescribeTags(%s): %w", instanceID, err)
	}

	tags := make([]Tag, 0, len(out.Tags))
	for _, t := range out.Tags {
		tags = append(tags, Tag{
			Key:   derefStr(t.Key),
			Value: derefStr(t.Value),
		})
	}
	return tags, nil
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func convertTags(in []types.Tag) []Tag {
	tags := make([]Tag, 0, len(in))
	for _, t := range in {
		tags = append(tags, Tag{
			Key:   derefStr(t.Key),
			Value: derefStr(t.Value),
		})
	}
	return tags
}

// SetTag sets a single tag key=value on the given instance.
func (c *Client) SetTag(ctx context.Context, instanceID, key, value string) error {
	if key == "" {
		return fmt.Errorf("tag key is required")
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		t := config.EC2Timeout(10 * time.Second)
		ctx, cancel = context.WithTimeout(ctx, t)
		defer cancel()
	}
	output.Debugf("EC2 CreateTags: %s %s=%s", instanceID, key, value)
	_, err := c.inner.CreateTags(ctx, &ec2.CreateTagsInput{
		Resources: []string{instanceID},
		Tags: []types.Tag{
			{Key: aws.String(key), Value: aws.String(value)},
		},
	})
	if err != nil {
		return fmt.Errorf("CreateTags(%s): %w", instanceID, err)
	}
	return nil
}
