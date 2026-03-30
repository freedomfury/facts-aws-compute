package ec2

import (
	"context"
	"fmt"

	imdsclient "github.com/facts/facts-aws-compute/internal/imds"
)

// IMDS base fields map — maps IMDS path to output JSON key.
var imdsFields = map[string]string{
	"ami-id":                      "AmiId",
	"instance-id":                 "InstanceId",
	"placement/region":            "Region",
	"placement/availability-zone": "AvailabilityZone",
	"local-ipv4":                  "PrivateIpAddress",
	"public-ipv4":                 "PublicIpAddress",
}

// DescribeInstanceFast builds InstanceInfo entirely from IMDS when possible,
// falling back to the EC2 API only for tags if IMDS tags are unavailable.
func DescribeInstanceFast(ctx context.Context, imds *imdsclient.Client, ec2Client *Client, instanceID, region string) (*InstanceInfo, error) {
	// 1. Fetch base fields from IMDS
	fields := make(map[string]string)
	for path, key := range imdsFields {
		val, err := imds.Get(ctx, "latest/meta-data/"+path)
		if err != nil {
			return nil, fmt.Errorf("IMDS fast path %s: %w", path, err)
		}
		fields[key] = val
	}

	info := &InstanceInfo{
		InstanceId:       fields["InstanceId"],
		AmiId:            fields["AmiId"],
		Region:           fields["Region"],
		AvailabilityZone: fields["AvailabilityZone"],
		PrivateIpAddress: fields["PrivateIpAddress"],
		PublicIpAddress:  fields["PublicIpAddress"],
	}

	// Override with explicit values if provided
	if instanceID != "" && instanceID != info.InstanceId {
		info.InstanceId = instanceID
	}
	if region != "" && region != info.Region {
		info.Region = region
	}

	// 2. Attempt tags from IMDS
	tags, err := fetchIMDSTags(ctx, imds)
	if err != nil {
		return nil, err
	}

	if tags != nil {
		// IMDS tags available — done, no EC2 API call needed
		info.Tags = tags
	} else {
		// IMDS tags not available — fall back to EC2 DescribeTags
		if ec2Client == nil {
			return nil, fmt.Errorf("IMDS tags unavailable and no EC2 client configured")
		}
		apiTags, err := ec2Client.DescribeTags(ctx, info.InstanceId)
		if err != nil {
			return nil, fmt.Errorf("fast path EC2 tag fallback: %w", err)
		}
		info.Tags = apiTags
	}

	return info, nil
}

// fetchIMDSTags attempts to read tags from IMDS at /latest/meta-data/tags/instance/.
// Returns (nil, nil) if the path is not available (404), indicating the caller
// should fall back to the EC2 API.
func fetchIMDSTags(ctx context.Context, imds *imdsclient.Client) ([]Tag, error) {
	keys, err := imds.List(ctx, "latest/meta-data/tags/instance")
	if err != nil {
		return nil, fmt.Errorf("IMDS tags list: %w", err)
	}
	if keys == nil {
		// 404 — tags not available via IMDS
		return nil, nil
	}

	tags := make([]Tag, 0, len(keys))
	for _, key := range keys {
		val, err := imds.Get(ctx, "latest/meta-data/tags/instance/"+key)
		if err != nil {
			return nil, fmt.Errorf("IMDS tag %s: %w", key, err)
		}
		tags = append(tags, Tag{Key: key, Value: val})
	}
	return tags, nil
}
