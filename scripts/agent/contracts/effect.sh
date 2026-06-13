#!/usr/bin/env bash
set -Eeuo pipefail

output="${1:-/workspace/evidence/implementation-contract.json}"

jq -n '
{
  schema_version: 1,
  kind: "implementation_contract",
  project: "ambience",
  feature_type: "effect",
  summary: "Add or change an ambience effect through the current Go/WASM pixel-grid runtime. The issue owns the aesthetic concept, event names, presets, and lifecycle intent; this contract owns the repo touchpoints.",
  read_first: [
    "AGENTS.md",
    "CLAUDE.md",
    "docs/effects-cookbook.md",
    "docs/dev-endpoints.md"
  ],
  reference_effects: [
    {
      slug: "burning-trees",
      files: [
        "sim/burning_trees.go",
        "sim/burning_trees_test.go",
        "cmd/ambience/effect_burning_trees.go"
      ],
      reason: "canonical current self-contained effect shape"
    },
    {
      slug: "magic-portal",
      files: [
        "sim/magic_portal.go",
        "cmd/ambience/effect_magic_portal.go",
        "cmd/ambience/web/sim.js"
      ],
      reason: "named preset and browser preset-map example"
    }
  ],
  slug_source: "Declare the public slug through ui_hint.route. The route must be /dev/<slug>; derive file names from that slug by replacing dashes with underscores.",
  scaffold: {
    command: "scripts/agent/scaffold-effect.sh <slug>",
    when: "use once for a new effect when the canonical sim/test/runtime files do not already exist",
    creates: [
      "sim/{effect_snake}.go",
      "sim/{effect_snake}_test.go",
      "cmd/ambience/effect_{effect_snake}.go"
    ],
    after_running: "replace the starter rendering/knobs/tests with the issue-owned behavior, then edit cmd/ambience-wasm/main.go"
  },
  required_file_templates: [
    "sim/{effect_snake}.go",
    "sim/{effect_snake}_test.go",
    "cmd/ambience/effect_{effect_snake}.go"
  ],
  required_touchpoints: [
    {
      path: "cmd/ambience-wasm/main.go",
      reason: "browser clients register Go-backed constructors through AmbienceSim.effects"
    }
  ],
  optional_touchpoints: [
    {
      path: "cmd/ambience/web/sim.js",
      when: "the issue declares named presets or browser-visible preset labels"
    },
    {
      path: "cmd/ambience/dev_random.go",
      when: "schema defaults or randomized dev configs make the first /dev render unreadable"
    }
  ],
  forbidden_paths: [
    {
      pattern: "cmd/ambience/web/effects/*",
      reason: "legacy standalone browser effects are retired; browser rendering is Go/WASM-backed"
    },
    {
      pattern: "cmd/ambience/web/*.html",
      reason: "new effects must not wire effect-specific scripts into HTML"
    },
    {
      pattern: "cmd/ambience/procedural_renderers.go",
      reason: "legacy shared renderer path; keep effect rendering in the effect-owned pixel grid"
    },
    {
      pattern: "scripts/agent/contracts/*",
      reason: "contract generation and validation are runner-owned, not effect implementation touchpoints"
    },
    {
      pattern: "scripts/glimmung-native/*",
      reason: "Glimmung wrapper scripts are runner-owned, not effect implementation touchpoints"
    }
  ],
  required_outputs: [
    "implementation",
    "ui_hint"
  ],
  validation_commands: [
    "go test ./sim -run <Effect>",
    "go test ./cmd/ambience -run '\''EveryEffectSchemaKnobRoundTrips|EveryLifecycleEffect|LiveEffectReplayAudit|BrowserEffectsComeFromWASMRuntime'\''",
    "GOOS=js GOARCH=wasm go build -o /tmp/ambience-test.wasm ./cmd/ambience-wasm",
    "go test ./...",
    "scripts/glimmung-native/agent-ci-feedback.sh publish-and-wait"
  ],
  behavior_evidence: {
    default_visual: "Run one local selfcheck for the default /dev/<slug> render.",
    terminal_lifecycle: "If intro or ending is implemented as terminal, add deterministic Go assertions and a /dev/observe lifecycle check."
  }
}
' >"$output"
