// Package skills holds the skills for AI agents, so that the finux binary
// can install them without the repository.
package skills

import "embed"

// Files has one folder for each skill, with its SKILL.md and references.
//
//go:embed finux-*
var Files embed.FS
