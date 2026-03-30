package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awsimds "github.com/aws/aws-sdk-go-v2/feature/ec2/imds"

	"github.com/facts/facts-aws-compute/internal/config"
	ec2client "github.com/facts/facts-aws-compute/internal/ec2"
	imdsclient "github.com/facts/facts-aws-compute/internal/imds"
	"github.com/facts/facts-aws-compute/internal/metadata"
	"github.com/facts/facts-aws-compute/internal/output"
)

const usage = `Usage: facts-aws-compute <command> [options]

Commands:
	ec2 describe-instances   Describe an EC2 instance ("fast" [default] or "full" mode)
		--fast                 Use IMDS for all fields, EC2 API fallback for tags only (default)
		--full                 Use EC2 API for all fields; --instance-id and --region required unless running on EC2
		--instance-id          Instance ID (used only with --full; auto-detected from IMDS if not provided)
		--region               AWS region (used only with --full; auto-detected from IMDS if not provided)
		--profile              AWS named profile (optional)
		--imds-timeout         IMDS timeout override in seconds
		--ec2-timeout          EC2 timeout override in seconds
	ec2 describe-tags        Describe tags for an EC2 instance
	ec2 set-tag              Set a tag key/value on an EC2 instance
	metadata get <path>      Get a single IMDS metadata value
	metadata dump            Dump IMDS metadata tree as JSON
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}

	// Quick global verbose detection (-v or --verbose) so debug logging
	// can be enabled without adding a complex global flag parser.
	for _, a := range os.Args[1:] {
		if a == "-v" || a == "--verbose" {
			output.SetVerbose(true)
			break
		}
	}

	// Build a filtered args list with global-only flags removed so the
	// routing logic can treat the first non-global token as the command.
	args := []string{os.Args[0]}
	for _, a := range os.Args[1:] {
		if a == "-v" || a == "--verbose" {
			continue
		}
		args = append(args, a)
	}

	ctx := context.Background()

	// If filtering removed the subcommand (e.g. only `-v` was provided),
	// show usage as we would when no command is given.
	if len(args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}

	// Route to subcommand
	switch args[1] {
	case "ec2":
		if len(args) < 3 {
			output.Fatal("ec2 requires a subcommand: describe-instances or describe-tags")
		}
		switch args[2] {
		case "describe-instances":
			cmdDescribeInstances(ctx, args[3:])
		case "describe-tags":
			cmdDescribeTags(ctx, args[3:])
		case "set-tag":
			cmdSetTag(ctx, args[3:])
		default:
			output.Fatalf("unknown ec2 subcommand: %s", args[2])
		}
	case "metadata":
		if len(args) < 3 {
			output.Fatal("metadata requires a subcommand: get or dump")
		}
		switch args[2] {
		case "get":
			cmdMetadataGet(ctx, args[3:])
		case "dump":
			cmdMetadataDump(ctx, args[3:])
		default:
			output.Fatalf("unknown metadata subcommand: %s", args[2])
		}
	case "-h", "--help", "help":
		fmt.Fprint(os.Stderr, usage)
		os.Exit(0)
	default:
		output.Fatalf("unknown command: %s", args[1])
	}
}

// ─── ec2 describe-instances ──────────────────────────────────────────

func cmdDescribeInstances(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("ec2 describe-instances", flag.ContinueOnError)
	instanceID := fs.String("instance-id", "", "Instance ID (used only with --full; auto-detected from IMDS if not provided)")
	region := fs.String("region", "", "AWS region (used only with --full; auto-detected from IMDS if not provided)")
	profile := fs.String("profile", "", "AWS named profile")
	fast := fs.Bool("fast", false, "Use IMDS for all fields, EC2 API fallback for tags only (default)")
	full := fs.Bool("full", false, "Use EC2 API for all fields; --instance-id and --region required unless running on EC2")
	imdsTimeout := fs.Int("imds-timeout", 0, "IMDS timeout override in seconds (env FACTS_IMDS_TIMEOUT or default used if 0)")
	ec2Timeout := fs.Int("ec2-timeout", 0, "EC2 timeout override in seconds (env FACTS_EC2_TIMEOUT or default used if 0)")

	if err := fs.Parse(args); err != nil {
		output.Fatalf("flag parse: %s", err)
	}

	if *imdsTimeout > 0 {
		config.SetIMDSTimeoutSeconds(*imdsTimeout)
	}
	if *ec2Timeout > 0 {
		config.SetEC2TimeoutSeconds(*ec2Timeout)
	}

	// Default to fast mode unless --full is specified
	useFast := *fast || !*full

	imds := mustIMDSClient(ctx)

	if useFast {
		// Fast mode: always use IMDS for instance ID and region
		ec2c := newEC2Client(ctx, "", *profile) // region will be set by DescribeInstanceFast
		info, err := ec2client.DescribeInstanceFast(ctx, imds, ec2c, "", "")
		if err != nil {
			output.Fatalf("fast path: %s", err)
		}
		if err := output.JSON(info); err != nil {
			output.Fatalf("json encode: %s", err)
		}
		return
	}

	// Full mode: require instance ID and region, fallback to IMDS if not provided
	iid := *instanceID
	reg := *region
	if iid == "" || reg == "" {
		// Try to get missing values from IMDS
		if iid == "" {
			iid = resolveInstanceID(ctx, imds, "")
		}
		if reg == "" {
			reg = resolveRegion(ctx, imds, "")
		}
	}
	if iid == "" || reg == "" {
		output.Fatal("--instance-id and --region are required for --full mode (unless running on EC2)")
	}
	ec2c := newEC2Client(ctx, reg, *profile)
	info, err := ec2c.DescribeInstance(ctx, iid, reg)
	if err != nil {
		output.Fatalf("describe-instances: %s", err)
	}
	if err := output.JSON(info); err != nil {
		output.Fatalf("json encode: %s", err)
	}
}

// ─── ec2 describe-tags ───────────────────────────────────────────────

func cmdDescribeTags(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("ec2 describe-tags", flag.ContinueOnError)
	instanceID := fs.String("instance-id", "", "Instance ID (auto-detected from IMDS if not provided)")
	region := fs.String("region", "", "AWS region (auto-detected if not provided)")
	profile := fs.String("profile", "", "AWS named profile")
	imdsTimeout := fs.Int("imds-timeout", 0, "IMDS timeout override in seconds (env FACTS_IMDS_TIMEOUT or default used if 0)")
	ec2Timeout := fs.Int("ec2-timeout", 0, "EC2 timeout override in seconds (env FACTS_EC2_TIMEOUT or default used if 0)")

	if err := fs.Parse(args); err != nil {
		output.Fatalf("flag parse: %s", err)
	}

	if *imdsTimeout > 0 {
		config.SetIMDSTimeoutSeconds(*imdsTimeout)
	}
	if *ec2Timeout > 0 {
		config.SetEC2TimeoutSeconds(*ec2Timeout)
	}

	imds := mustIMDSClient(ctx)
	iid := resolveInstanceID(ctx, imds, *instanceID)
	reg := resolveRegion(ctx, imds, *region)

	ec2c := newEC2Client(ctx, reg, *profile)
	tags, err := ec2c.DescribeTags(ctx, iid)
	if err != nil {
		output.Fatalf("describe-tags: %s", err)
	}

	result := struct {
		Tags []ec2client.Tag `json:"Tags"`
	}{Tags: tags}

	if err := output.JSON(result); err != nil {
		output.Fatalf("json encode: %s", err)
	}
}

// ─── ec2 set-tag ─────────────────────────────────────────────────────

func cmdSetTag(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("ec2 set-tag", flag.ContinueOnError)
	instanceID := fs.String("instance-id", "", "Instance ID (auto-detected from IMDS if not provided)")
	region := fs.String("region", "", "AWS region (auto-detected if not provided)")
	profile := fs.String("profile", "", "AWS named profile")
	key := fs.String("key", "", "Tag key to set (required)")
	value := fs.String("value", "", "Tag value to set (required)")
	imdsTimeout := fs.Int("imds-timeout", 0, "IMDS timeout override in seconds (env FACTS_IMDS_TIMEOUT or default used if 0)")
	ec2Timeout := fs.Int("ec2-timeout", 0, "EC2 timeout override in seconds (env FACTS_EC2_TIMEOUT or default used if 0)")

	if err := fs.Parse(args); err != nil {
		output.Fatalf("flag parse: %s", err)
	}

	if *imdsTimeout > 0 {
		config.SetIMDSTimeoutSeconds(*imdsTimeout)
	}
	if *ec2Timeout > 0 {
		config.SetEC2TimeoutSeconds(*ec2Timeout)
	}

	if *key == "" {
		output.Fatal("--key is required")
	}
	if *value == "" {
		output.Fatal("--value is required")
	}

	imds := mustIMDSClient(ctx)
	iid := resolveInstanceID(ctx, imds, *instanceID)
	reg := resolveRegion(ctx, imds, *region)

	ec2c := newEC2Client(ctx, reg, *profile)
	if err := ec2c.SetTag(ctx, iid, *key, *value); err != nil {
		output.Fatalf("set-tag: %s", err)
	}

	result := struct {
		Instance string `json:"Instance"`
		Key      string `json:"Key"`
		Value    string `json:"Value"`
	}{Instance: iid, Key: *key, Value: *value}

	if err := output.JSON(result); err != nil {
		output.Fatalf("json encode: %s", err)
	}
}

// ─── metadata get ────────────────────────────────────────────────────

func cmdMetadataGet(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("metadata get", flag.ContinueOnError)
	imdsTimeout := fs.Int("imds-timeout", 0, "IMDS timeout override in seconds (env FACTS_IMDS_TIMEOUT or default used if 0)")

	if err := fs.Parse(args); err != nil {
		output.Fatalf("flag parse: %s", err)
	}

	if *imdsTimeout > 0 {
		config.SetIMDSTimeoutSeconds(*imdsTimeout)
	}

	if fs.NArg() < 1 {
		output.Fatal("metadata get requires a path argument, e.g. /latest/meta-data/instance-id")
	}

	path := fs.Arg(0)
	imds := mustIMDSClient(ctx)

	// Strip leading / for the SDK
	trimmed := strings.TrimPrefix(path, "/")
	val, err := imds.Get(ctx, trimmed)
	if err != nil {
		output.Fatalf("metadata get %s: %s", path, err)
	}
	if val == "" {
		output.Fatalf("metadata get %s: path not found or empty", path)
	}

	result := struct {
		Path  string `json:"path"`
		Value string `json:"value"`
	}{Path: path, Value: val}

	if err := output.JSON(result); err != nil {
		output.Fatalf("json encode: %s", err)
	}
}

// ─── metadata dump ───────────────────────────────────────────────────

func cmdMetadataDump(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("metadata dump", flag.ContinueOnError)
	path := fs.String("path", "/latest/meta-data/", "Subtree path to dump")
	imdsTimeout := fs.Int("imds-timeout", 0, "IMDS timeout override in seconds (env FACTS_IMDS_TIMEOUT or default used if 0)")

	if err := fs.Parse(args); err != nil {
		output.Fatalf("flag parse: %s", err)
	}

	if *imdsTimeout > 0 {
		config.SetIMDSTimeoutSeconds(*imdsTimeout)
	}

	imds := mustIMDSClient(ctx)
	tree, err := metadata.Walk(ctx, imds, *path)
	if err != nil {
		output.Fatalf("metadata dump: %s", err)
	}
	if tree == nil {
		output.Fatalf("metadata dump: no data at path %s", *path)
	}
	if err := output.JSON(tree); err != nil {
		output.Fatalf("json encode: %s", err)
	}
}

// ─── helpers ─────────────────────────────────────────────────────────

func mustIMDSClient(ctx context.Context) *imdsclient.Client {
	output.Debugf("loading AWS config for IMDS")
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		output.Fatalf("failed to load AWS config for IMDS: %s", err)
	}
	return imdsclient.New(awsimds.NewFromConfig(cfg))
}

func newEC2Client(ctx context.Context, region, profile string) *ec2client.Client {
	output.Debugf("newEC2Client region=%s profile=%s", region, profile)
	var awsOpts []func(*awsconfig.LoadOptions) error
	awsOpts = append(awsOpts, awsconfig.WithRegion(region))
	if profile != "" {
		awsOpts = append(awsOpts, awsconfig.WithSharedConfigProfile(profile))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsOpts...)
	if err != nil {
		output.Fatalf("failed to load AWS config: %s", err)
	}
	return ec2client.New(cfg)
}

func resolveInstanceID(ctx context.Context, imds *imdsclient.Client, flagValue string) string {
	if flagValue != "" {
		output.Debugf("resolveInstanceID: using flag %s", flagValue)
		return flagValue
	}
	val, err := imds.GetRequired(ctx, "latest/meta-data/instance-id")
	if err != nil {
		output.Fatalf("failed to detect instance ID: no --instance-id flag and IMDS unavailable: %s", err)
	}
	output.Debugf("resolveInstanceID: detected %s via IMDS", val)
	return val
}

func resolveRegion(ctx context.Context, imds *imdsclient.Client, flagValue string) string {
	if flagValue != "" {
		output.Debugf("resolveRegion: using flag %s", flagValue)
		return flagValue
	}
	if env := os.Getenv("AWS_DEFAULT_REGION"); env != "" {
		output.Debugf("resolveRegion: using AWS_DEFAULT_REGION=%s", env)
		return env
	}
	val, err := imds.GetRequired(ctx, "latest/meta-data/placement/region")
	if err != nil {
		output.Fatalf("failed to detect region: no --region flag, no AWS_DEFAULT_REGION, and IMDS unavailable: %s", err)
	}
	output.Debugf("resolveRegion: detected %s via IMDS", val)
	return val
}
