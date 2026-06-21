// Package migrationguard carries the retirement guard for ambience's shell
// run-harness migration. Its test fails if any retired shell-harness path or
// sentinel symbol reappears in live repo files — enforcing the migration policy
// (delete the old path end to end; no parallel path). The Go run-harness under
// glimmung-harness/ is the only sanctioned step-producer surface for ambience.
package migrationguard
