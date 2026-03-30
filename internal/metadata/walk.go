package metadata

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	imdsclient "github.com/facts/facts-aws-compute/internal/imds"
	"github.com/facts/facts-aws-compute/internal/output"
)

// Walk recursively walks the IMDS tree starting at root and returns
// a nested map representing the directory structure as JSON-friendly data.
func Walk(ctx context.Context, client *imdsclient.Client, root string) (map[string]interface{}, error) {
	// Normalize: ensure root ends with /
	if !strings.HasSuffix(root, "/") {
		root += "/"
	}
	output.Debugf("metadata Walk root=%s", root)
	// Strip leading / for the SDK (it adds its own)
	path := strings.TrimPrefix(root, "/")

	return walkPath(ctx, client, path)
}

func walkPath(ctx context.Context, client *imdsclient.Client, path string) (map[string]interface{}, error) {
	keys, err := client.List(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("walk list %s: %w", path, err)
	}
	if keys == nil {
		return nil, nil
	}

	output.Debugf("walkPath %s -> %d keys", path, len(keys))

	result := make(map[string]interface{})
	for _, key := range keys {
		// Skip user-data entirely
		if key == "user-data" || key == "user-data/" {
			output.Debugf("skipping user-data at %s", path)
			continue
		}

		if strings.HasSuffix(key, "/") {
			// Directory — recurse
			name := strings.TrimSuffix(key, "/")
			sub, err := walkPath(ctx, client, path+key)
			if err != nil {
				// Non-fatal: skip subtrees that fail
				output.Debugf("walkPath %s: skipping subtree %s: %v", path, key, err)
				continue
			}
			if sub != nil {
				result[name] = sub
			}
		} else {
			// Leaf — fetch value
			val, err := client.Get(ctx, path+key)
			if err != nil {
				output.Debugf("walkPath %s: failed to get %s: %v", path, key, err)
				continue
			}
			// Skip binary content
			if !utf8.ValidString(val) {
				output.Debugf("walkPath %s%s: skipping binary content", path, key)
				continue
			}
			output.Debugf("walkPath %s%s: leaf -> %d bytes", path, key, len(val))
			result[key] = val
		}
	}
	return result, nil
}
