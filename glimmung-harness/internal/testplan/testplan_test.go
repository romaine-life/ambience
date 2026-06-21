package testplan

import (
	"reflect"
	"testing"
)

func TestRegistrySlugs(t *testing.T) {
	want := []string{"clone", "collect", "emit", "finalize", "prepare", "run-test-plan"}
	got := Registry().Slugs()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("test-plan slugs=%v want %v", got, want)
	}
}
