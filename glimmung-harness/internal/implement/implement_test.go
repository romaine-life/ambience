package implement

import (
	"reflect"
	"testing"
)

func TestRegistrySlugs(t *testing.T) {
	want := []string{
		"clone", "collect", "emit", "ensure-draft-pr", "finalize",
		"prepare", "prepare-draft-pr-branch", "push-branch",
		"rebuild-env", "run-implementation", "wait-pr-checks",
	}
	got := Registry().Slugs()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("implement slugs=%v want %v", got, want)
	}
}

func TestUIHintString(t *testing.T) {
	got := uiHintString(`{"status":"pass","ui_hint":{"menu_label":"x","route":"/dev/x"}}`)
	if got != `{"menu_label":"x","route":"/dev/x"}` {
		t.Fatalf("uiHintString=%q", got)
	}
	if uiHintString(`{"status":"pass"}`) != "" {
		t.Fatal("missing ui_hint should yield empty string")
	}
}

func TestSplitSlug(t *testing.T) {
	owner, repo := splitSlug("romaine-life/ambience")
	if owner != "romaine-life" || repo != "ambience" {
		t.Fatalf("splitSlug=%q/%q", owner, repo)
	}
}
