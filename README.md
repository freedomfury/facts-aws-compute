# facts-aws-compute — Minimal EC2 Introspection CLI

A purpose-built, statically compiled Go binary that replaces a subset of the AWS CLI
for EC2 instance metadata and tag queries. Part of the FACTS bootstrap system.

## Build

```bash
make build
# produces: ./dist/facts-aws-compute (static linux/amd64 binary)
```

## Commands

```bash
# Describe an EC2 instance (auto-detects from IMDS)
facts-aws-compute ec2-describe-instances

# Fast path — IMDS first, EC2 API only for tags if needed
facts-aws-compute ec2-describe-instances --fast

# Explicit instance + region
facts-aws-compute ec2-describe-instances --instance-id i-0abc123 --region us-east-1

# Tags only
facts-aws-compute ec2-describe-tags

# Set a tag key/value on the instance (auto-detects instance-id via IMDS)
facts-aws-compute ec2-set-tag --key environment --value production

# Single metadata value
facts-aws-compute metadata-get /latest/meta-data/instance-id

# Dump full metadata tree
facts-aws-compute metadata-dump

# Dump subtree
facts-aws-compute metadata-dump --path /latest/meta-data/placement/
```

## Output

All output is JSON to stdout. All errors are JSON to stderr.
