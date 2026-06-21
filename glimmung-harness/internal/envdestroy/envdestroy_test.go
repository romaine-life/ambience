package envdestroy

import (
	"reflect"
	"testing"
)

func TestRegistrySlugs(t *testing.T) {
	want := []string{
		"cleanup-issue-branches", "delete-namespace", "describe-pre-teardown",
		"emit", "uninstall-helm-release",
	}
	got := Registry().Slugs()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("env-destroy slugs=%v want %v", got, want)
	}
}
