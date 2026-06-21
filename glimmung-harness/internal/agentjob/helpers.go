package agentjob

import (
	"encoding/base64"
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

// caConfigMapFromSecret mirrors copy_github_policy_ca's jq: decode the Secret's
// base64 ca.crt into a plain ConfigMap targeted at the slot namespace.
func caConfigMapFromSecret(secretJSON, namespace, configmapName string) (string, error) {
	var secret struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal([]byte(secretJSON), &secret); err != nil {
		return "", fmt.Errorf("parse policy CA secret: %w", err)
	}
	encoded, ok := secret.Data["ca.crt"]
	if !ok {
		return "", fmt.Errorf("policy CA secret has no ca.crt")
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode policy CA ca.crt: %w", err)
	}
	cm := map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]any{"name": configmapName, "namespace": namespace},
		"data":       map[string]any{"ca.crt": string(decoded)},
	}
	out, err := json.Marshal(cm)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
