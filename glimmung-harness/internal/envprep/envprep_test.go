package envprep

import (
	"reflect"
	"testing"
)

// The env-prep registry must carry exactly the slugs env-prep.sh dispatched, in
// the same set — the workflow re-registration binds each slug to this binary.
func TestRegistrySlugs(t *testing.T) {
	want := []string{
		"build-validation-image",
		"check-validation-env",
		"clone-repo",
		"deploy-validation-env",
		"emit-env-outputs",
		"push-validation-image",
		"reap-slot-conflicts",
	}
	got := Registry().Slugs() // sorted
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("env-prep slugs=%v want %v", got, want)
	}
}
