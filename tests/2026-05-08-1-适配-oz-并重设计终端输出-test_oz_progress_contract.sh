#!/usr/bin/env bash
# Run the oz migration and compact progress contract tests for this change.

set -euo pipefail

go test ./internal/app -run 'TestListChangesUsesOzListJSON|TestChangeTasksDoneUsesOzStatus|TestBundledOzSkillPromptsDelegateToSkills|TestStageChecklistLinesHideFutureFixRounds|TestStageChecklistLinesMergesFixAndFollowupReview|TestRunStatusAndAbortJSONUseSpecificRun'
