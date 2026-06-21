package agentjob

import (
	"encoding/json"
	"fmt"
	"os"
)

func fileNonEmpty(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Size() > 0
}

// rewriteConfigMapNamespace mirrors copy_claude_ca's jq: strip server-managed
// metadata and retarget the ConfigMap at the slot namespace. kubectl apply
// accepts the JSON document directly.
func rewriteConfigMapNamespace(srcJSON, namespace string) (string, error) {
	var doc map[string]any
	if err := json.Unmarshal([]byte(srcJSON), &doc); err != nil {
		return "", fmt.Errorf("parse source configmap: %w", err)
	}
	meta, _ := doc["metadata"].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
	}
	for _, k := range []string{"annotations", "uid", "resourceVersion", "generation", "creationTimestamp", "managedFields"} {
		delete(meta, k)
	}
	meta["namespace"] = namespace
	doc["metadata"] = meta
	out, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
