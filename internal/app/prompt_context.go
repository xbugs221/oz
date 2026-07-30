// Package app builds sealed-run prompt templates and stable prompt context paths.
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// promptSnapshot stores the effective prompt bodies frozen for one sealed run.
type promptSnapshot struct {
	Prompts map[string]string `yaml:"prompts"`
}

// promptTemplateContext is the data exposed to named prompt templates.
type promptTemplateContext struct {
	RunID                        string
	RunDirectory                 string
	ChangeName                   string
	Stage                        string
	StageKind                    string
	Iteration                    int
	MaxReviewIterations          int
	MaxRepairIterations          int
	StatePath                    string
	ChangePath                   string
	AcceptancePath               string
	AcceptanceSummaryPath        string
	ReviewPath                   string
	RepairPath                   string
	HasRepairCheckpoint          bool
	QAPath                       string
	FixSummaryPath               string
	PreviousReviewPaths          []string
	PreviousRepairPaths          []string
	PreviousQAPaths              []string
	PreviousFixSummaryPaths      []string
	PreviousReviewCount          int
	PreviousRepairCount          int
	PreviousFixSummaryCount      int
	LatestPreviousReviewPath     string
	LatestPreviousRepairPath     string
	LatestPreviousQAPath         string
	LatestPreviousRepairFile     string
	LatestPreviousQAFile         string
	LatestPreviousFixSummaryPath string
	HasPreviousReview            bool
	HasPreviousRepair            bool
	HasPreviousQA                bool
	HasPreviousFixSummary        bool
	PlanningContextPath          string
	ParallelContextPath          string
	ParallelReviewPath           string
	ParallelQAPath               string
	HasPlanningContext           bool
	HasParallelContext           bool
	HasParallelReview            bool
	HasParallelQA                bool
	DeliverySummaryPath          string
	BaselineHead                 string
	RoleSessionKey               string
	RoleSessionID                string
	HasRoleSession               bool
	IsFirstRoleTurn              bool
	IsRepairConfirmation         bool
	FixEscalated                 bool
	FixEscalationReasoning       string
	ConsecutiveReviewFailures    int
	RepeatedFindingTitles        []string
	RepairMode                   string
	SourceQAArtifact             string
	QAFindingSummaries           []string
	FailedAcceptanceIDs          []string
}

// promptForStage reads and renders the YAML prompt for a sealed stage.
func promptForStage(repo string, state State) (string, error) {
	if state.ChangeName != "" {
		if err := validateChangeNameForPath(state.ChangeName); err != nil {
			return "", err
		}
	}
	name, err := promptNameForStage(state.Stage)
	if err != nil {
		return "", err
	}
	var templateText string
	if state.RunID != "" {
		templateText, err = runPromptTemplate(repo, state.RunID, name)
		if err != nil {
			return "", err
		}
	} else {
		config := DefaultWorkflowConfig()
		if state.Workflow.Prompts != nil {
			config = state.Workflow
		} else if loaded, loadErr := LoadWorkflowConfig(repo); loadErr == nil {
			config = loaded
		}
		templateText, err = promptForName(config, name)
		if err != nil {
			return "", err
		}
	}
	context, err := promptContext(repo, state)
	if err != nil {
		return "", err
	}
	prompt, err := renderPromptTemplate(name, templateText, context)
	if err != nil {
		return "", err
	}
	if failurePrompt := validationFailurePrompt(repo, state); failurePrompt != "" {
		prompt = failurePrompt
	}
	if usesQualityLoop(state.Workflow) {
		prompt = appendQualityLoopPrompt(prompt, context)
	}
	return prompt, nil
}

// appendQualityLoopPrompt adds the mode-specific scope and deterministic exit contract.
func appendQualityLoopPrompt(prompt string, context promptTemplateContext) string {
	var block strings.Builder
	block.WriteString("\n\n# 动态质量循环合同\n\n")
	fmt.Fprintf(&block, "- 当前阶段：`%s`（第 `%d` 轮）；运行目录：`%s`；diff baseline：`%s`。\n", context.Stage, context.Iteration, context.RunDirectory, context.BaselineHead)
	stageKind := context.Stage
	if parsed, err := parseWorkflowStage(context.Stage); err == nil {
		stageKind = parsed.Kind
	}
	switch stageKind {
	case workflowStagePlanning:
		block.WriteString("- 规划必须定义 `delivery_report`：用用户语言写收益、验收操作、可见结果和直接证据；修复类场景还要绑定同一场景下可理解且不同的前后证据。\n")
	case workflowStageExecution:
		block.WriteString("- 执行完成前必须产出真实可打开的演示媒体；禁止用 echo/printf、硬编码字符串、退出码或测试通过字样冒充用户证据。\n")
	case workflowStageQA:
		block.WriteString("- QA 必须逐项填写 `user_acceptance[]`，用普通用户语言记录实际看到的行为并引用交付场景证据；命令、退出码、哈希、HTTP 200 或元素存在不能替代用户结果。\n")
	}
	switch context.RepairMode {
	case "pre_qa_audit":
		block.WriteString("- 若确定缺少环境前置条件，在 artifact evidence 中写 `blocked_environment: VARIABLE_OR_PATH`；只写变量名/路径，不写密钥值。\n")
		block.WriteString("- 执行、自查和定向修复期间不得创建 git commit；完整交付提交只能由归档阶段创建。\n")
		block.WriteString("- 移交独立 QA 前，按仓库已有显式入口运行提交前钩子，不得创建临时 commit；吸收改动后，再次运行不再修改文件才可移交。钩子稳定后重新运行受影响测试、全部 required tests 和 validation commands。\n")
		fmt.Fprintf(&block, "- 模式：`pre_qa_audit`；写入：`audit-%d.json`（相对运行目录）。\n- 在运行目录中运行：`oz flow validate-repair --artifact \"audit-%d.json\" --json`。\n- 全量检查当前提案的 acceptance、完整 diff、源码、测试与证据；实际运行并确认 demo 覆盖目标能力，核对上一检查点的不可变证据快照（如有），只产出 `test-results/**` 临时源并交由本阶段后置门禁封存，不得写入 `tests/evidence/proposals/<change>/**` 提交级证据包；本轮零新问题且 required tests 通过才可移交独立 QA。\n", context.Iteration, context.Iteration)
	case "qa_targeted_repair":
		block.WriteString("- 若确定缺少环境前置条件，在 artifact evidence 中写 `blocked_environment: VARIABLE_OR_PATH`；只写变量名/路径，不写密钥值。\n")
		block.WriteString("- 执行、自查和定向修复期间不得创建 git commit；完整交付提交只能由归档阶段创建。\n")
		block.WriteString("- 移交独立 QA 前，按仓库已有显式入口运行提交前钩子，不得创建临时 commit；吸收改动后，再次运行不再修改文件才可移交。钩子稳定后重新运行受影响测试、全部 required tests 和 validation commands。\n")
		fmt.Fprintf(&block, "- 模式：`qa_targeted_repair`；写入：`targeted-repair-%d.json`（相对运行目录）。\n- 在运行目录中运行：`oz flow validate-repair --artifact \"targeted-repair-%d.json\" --json`。\n- 仅处理最新 QA findings 及直接相关回归；来源 QA：`qa-%d.json`。逐项产出同一用户场景下可理解且不同的修复前后证据；命令输出、退出码、哈希或硬编码字符串不算用户证据。\n", context.Iteration, context.Iteration, context.Iteration)
		if len(context.QAFindingSummaries) > 0 {
			block.WriteString("- 最新 QA findings：\n")
			for _, finding := range context.QAFindingSummaries {
				fmt.Fprintf(&block, "  - %s\n", finding)
			}
		}
		if len(context.FailedAcceptanceIDs) > 0 {
			fmt.Fprintf(&block, "- 失败 acceptance IDs：`%s`\n", strings.Join(context.FailedAcceptanceIDs, "`, `"))
		}
		block.WriteString("- 移交前必须复跑失败测试、全部 required tests 和 validation commands；任何失败或过期结果都留在本阶段。\n")
	}
	if context.Stage == workflowStageArchive {
		block.WriteString("- 归档是最终 QA 后的只读边界：仅执行归档 skill 的机械移动并写入 delivery-summary.md；长期规格与规格测试必须已在最终 QA 前完成。\n")
		block.WriteString("- 引擎会生成 `tests/evidence/proposals/<change>/DELIVERY.md`；归档代理必须核对用户收益、验收步骤、实测结果与直接证据均可理解、可打开，再将整包随本次归档提交。\n")
		block.WriteString("- 最终包仍须位于 `tests/evidence/proposals/<change>/**`、不得命中 git ignore，也不得从 `test-results/**` 重建；归档后严禁编辑，按引擎产物原样暂存。\n")
		block.WriteString("- 必须从 state 封存的 `delivery_base_head` 新建且只新建一个完整交付 commit，使实现、归档提案与最终证据同属 HEAD；禁止 amend、squash 或沿用执行/自查阶段的提交。\n")
		block.WriteString("- 不得在最终 QA 后首次触发会改写文件的提交前钩子；只允许复核上一自查阶段已确认幂等的同一入口。若仍产生改动，停止归档并返回自查与 QA，不得吸收为已测试内容。\n")
		block.WriteString("- 归档命令返回后，严禁编辑、格式化或恢复提案目录及任何源码；可以原样暂存/提交命令结果，但不得为工作区干净改写内容，差异必须交由只读门禁判定。\n")
	}
	return prompt + block.String()
}

// promptForName reads a named prompt from the effective YAML config.
func promptForName(config WorkflowConfig, name string) (string, error) {
	key, err := promptKeyForName(name)
	if err != nil {
		return "", err
	}
	body := config.Prompts[key]
	if body == "" {
		return "", fmt.Errorf("配置缺少 prompts.%s", key)
	}
	return body, nil
}

// promptKeyForName resolves a prompt name to its workflow YAML key.
func promptKeyForName(name string) (string, error) {
	role, ok := roleByPromptName(name)
	if !ok {
		return "", fmt.Errorf("未知 prompt %q", name)
	}
	return role.PromptKey, nil
}

// runPromptTemplate reads the prompt snapshot saved when a sealed run starts.
func runPromptTemplate(repo, runID, name string) (string, error) {
	key, keyErr := promptKeyForName(name)
	if keyErr != nil {
		return "", keyErr
	}
	snapshotPath := filepath.Join(runDir(repo, runID), "prompt-snapshot.yaml")
	data, err := os.ReadFile(snapshotPath)
	if err == nil {
		var snapshot promptSnapshot
		if err := yaml.Unmarshal(data, &snapshot); err != nil {
			return "", fmt.Errorf("读取 prompt 快照 %s 失败: %w", snapshotPath, err)
		}
		body := snapshot.Prompts[key]
		if body == "" {
			return "", fmt.Errorf("prompt 快照缺少 prompts.%s", key)
		}
		return body, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	return "", fmt.Errorf("run %s 缺少 prompt 快照 prompt-snapshot.yaml", runID)
}

// snapshotRunPrompts freezes sealed-run prompts so resume cannot drift.
func snapshotRunPrompts(repo, runID string) error {
	root := runDir(repo, runID)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	config, err := LoadWorkflowConfig(repo)
	if err != nil {
		return err
	}
	snapshot := promptSnapshot{Prompts: map[string]string{}}
	for _, key := range promptKeysForWorkflow(config) {
		body := config.Prompts[key]
		if body == "" {
			return fmt.Errorf("配置缺少 prompts.%s", key)
		}
		snapshot.Prompts[key] = body
	}
	data, err := yaml.Marshal(snapshot)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "prompt-snapshot.yaml"), data, 0o644)
}

// promptKeysForWorkflow returns only prompts used by the active state-machine generation.
func promptKeysForWorkflow(config WorkflowConfig) []string {
	if usesQualityLoop(config) || usesRepairWorkflow(config) {
		return []string{"planning", "execution", "repair", "qa", "archive"}
	}
	return []string{"planning", "execution", "review", "qa", "fix", "archive"}
}

// renderPromptTemplate injects run metadata and fails on unknown template variables.
func renderPromptTemplate(name, body string, context promptTemplateContext) (string, error) {
	tmpl, err := template.New(name).Option("missingkey=error").Parse(body)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	if err := tmpl.Execute(&out, context); err != nil {
		return "", err
	}
	return out.String(), nil
}

// promptContext computes stable run paths for the current workflow stage.
func promptContext(repo string, state State) (promptTemplateContext, error) {
	ensureWorkflowConfig(&state)
	kind := stageKind(state.Stage)
	iteration, err := stageIteration(state.Stage)
	if err != nil {
		return promptTemplateContext{}, err
	}
	runPath := runDir(repo, state.RunID)
	sealedAcceptancePath := acceptancePath(repo, state.ChangeName)
	if candidate := filepath.Join(runPath, "acceptance.json"); state.RunID != "" {
		if usesQualityLoop(state.Workflow) {
			if _, err := readAcceptanceForState(repo, state); err != nil {
				return promptTemplateContext{}, fmt.Errorf("quality loop 缺少有效的封存 acceptance %s: %w", candidate, err)
			}
			sealedAcceptancePath = candidate
		} else if fileExists(candidate) {
			sealedAcceptancePath = candidate
		}
	}
	roleSessionKey, roleSessionID := promptRoleSession(state)
	context := promptTemplateContext{
		RunID:                   state.RunID,
		RunDirectory:            runPath,
		ChangeName:              state.ChangeName,
		Stage:                   state.Stage,
		StageKind:               kind,
		Iteration:               iteration,
		MaxReviewIterations:     state.Workflow.MaxReviewIterations,
		MaxRepairIterations:     state.Workflow.MaxRepairIterations,
		StatePath:               filepath.Join(runPath, "state.json"),
		ChangePath:              "docs/changes/" + state.ChangeName,
		AcceptancePath:          sealedAcceptancePath,
		AcceptanceSummaryPath:   sealedAcceptancePath,
		DeliverySummaryPath:     filepath.Join(runPath, "delivery-summary.md"),
		BaselineHead:            state.BaselineHead,
		RoleSessionKey:          roleSessionKey,
		RoleSessionID:           roleSessionID,
		HasRoleSession:          roleSessionID != "",
		IsFirstRoleTurn:         roleSessionID == "",
		IsRepairConfirmation:    kind == workflowStageRepair && state.RepairConfirmationPending,
		PreviousReviewPaths:     []string{},
		PreviousRepairPaths:     []string{},
		PreviousQAPaths:         []string{},
		PreviousFixSummaryPaths: []string{},
		RepeatedFindingTitles:   []string{},
	}
	if usesQualityLoop(state.Workflow) {
		if kind == workflowStageAudit {
			context.RepairMode = "pre_qa_audit"
			context.RepairPath = filepath.Join(runPath, fmt.Sprintf("audit-%d.json", iteration))
			context.HasRepairCheckpoint = true
			if err := appendPreviousQualityArtifacts(&context, state, runPath, "audit_", iteration); err != nil {
				return promptTemplateContext{}, err
			}
		}
		if kind == workflowStageTargetedRepair {
			context.RepairMode = "qa_targeted_repair"
			context.RepairPath = filepath.Join(runPath, fmt.Sprintf("targeted-repair-%d.json", iteration))
			context.HasRepairCheckpoint = true
			context.SourceQAArtifact = filepath.Join(runPath, fmt.Sprintf("qa-%d.json", iteration))
			qa, readErr := ReadQA(context.SourceQAArtifact)
			if readErr != nil {
				return promptTemplateContext{}, fmt.Errorf("定向修复缺少有效的来源 QA artifact %s: %w", context.SourceQAArtifact, readErr)
			}
			if !QANeedsFix(qa) {
				return promptTemplateContext{}, fmt.Errorf("定向修复来源 QA artifact %s 未声明 needs_fix", context.SourceQAArtifact)
			}
			acceptance, acceptanceErr := readAcceptanceForState(repo, state)
			if acceptanceErr != nil {
				return promptTemplateContext{}, acceptanceErr
			}
			if err := ValidateQAAgainstAcceptance(qa, acceptance); err != nil {
				return promptTemplateContext{}, fmt.Errorf("定向修复来源 QA artifact %s 未覆盖封存 acceptance: %w", context.SourceQAArtifact, err)
			}
			expectedSource := fmt.Sprintf("qa-%d.json", iteration)
			if state.QualityLoop.SourceQAArtifact != expectedSource ||
				state.QualityLoop.SourceQAHash == "" ||
				qaArtifactContentHash(qa) != state.QualityLoop.SourceQAHash {
				return promptTemplateContext{}, fmt.Errorf("定向修复来源 QA artifact %s 与已封存完整内容不一致", context.SourceQAArtifact)
			}
			if state.QualityLoop.FindingFingerprint != "" &&
				qaFindingFingerprint(qa) != state.QualityLoop.FindingFingerprint {
				return promptTemplateContext{}, fmt.Errorf("定向修复来源 QA artifact %s 与已封存 finding fingerprint 不一致", context.SourceQAArtifact)
			}
			sourceStage := fmt.Sprintf("qa_%d", iteration)
			if !qualityLoopTrustedSourceQA(repo, state, sourceStage, qa) {
				return promptTemplateContext{}, fmt.Errorf(
					"定向修复来源 QA artifact %s 未通过已测试输入信任校验",
					context.SourceQAArtifact,
				)
			}
			context.PreviousQAPaths = append(context.PreviousQAPaths, context.SourceQAArtifact)
			context.HasPreviousQA = true
			context.LatestPreviousQAPath = context.SourceQAArtifact
			if err := appendPreviousQualityArtifacts(&context, state, runPath, "targeted_repair_", iteration); err != nil {
				return promptTemplateContext{}, err
			}
			context.QAFindingSummaries = qaFindingSummaries(qa)
			context.FailedAcceptanceIDs = failedAcceptanceIDs(qa)
		}
		if kind == workflowStageQA {
			context.QAPath = filepath.Join(runPath, fmt.Sprintf("qa-%d.json", iteration))
			checkpoint, checkpointErr := latestQualityLoopRepairCheckpoint(state, runPath, iteration)
			if checkpointErr != nil {
				return promptTemplateContext{}, checkpointErr
			}
			if checkpoint != "" {
				context.RepairPath = checkpoint
				context.HasRepairCheckpoint = true
			}
		}
	}
	escalation, err := fixEscalation(repo, state)
	if err != nil {
		return promptTemplateContext{}, err
	}
	if escalation.Enabled {
		context.FixEscalated = escalation.Enabled
		context.FixEscalationReasoning = escalation.Reasoning
		context.ConsecutiveReviewFailures = escalation.ConsecutiveFailures
		context.RepeatedFindingTitles = escalation.RepeatedFindingTitles
	}
	if iteration > 0 && !usesQualityLoop(state.Workflow) {
		if usesRepairWorkflow(state.Workflow) && state.Workflow.MaxRepairIterations > 0 {
			context.RepairPath = filepath.Join(runPath, fmt.Sprintf("repair-%d.json", iteration))
			context.HasRepairCheckpoint = true
		}
		context.ReviewPath = filepath.Join(runPath, fmt.Sprintf("review-%d.json", iteration))
		context.QAPath = filepath.Join(runPath, fmt.Sprintf("qa-%d.json", iteration))
		context.FixSummaryPath = filepath.Join(runPath, fmt.Sprintf("fix-%d-summary.md", iteration))
		for i := 1; i < iteration; i++ {
			context.PreviousRepairPaths = append(context.PreviousRepairPaths, filepath.Join(runPath, fmt.Sprintf("repair-%d.json", i)))
			context.PreviousReviewPaths = append(context.PreviousReviewPaths, filepath.Join(runPath, fmt.Sprintf("review-%d.json", i)))
			context.PreviousFixSummaryPaths = append(context.PreviousFixSummaryPaths, filepath.Join(runPath, fmt.Sprintf("fix-%d-summary.md", i)))
			if state.Stages[fmt.Sprintf("qa_%d", i)] == "completed" {
				context.PreviousQAPaths = append(context.PreviousQAPaths, filepath.Join(runPath, fmt.Sprintf("qa-%d.json", i)))
			}
		}
		context.PreviousReviewCount = len(context.PreviousReviewPaths)
		context.PreviousRepairCount = len(context.PreviousRepairPaths)
		context.PreviousFixSummaryCount = len(context.PreviousFixSummaryPaths)
		context.HasPreviousReview = context.PreviousReviewCount > 0
		context.HasPreviousRepair = context.PreviousRepairCount > 0
		context.HasPreviousFixSummary = context.PreviousFixSummaryCount > 0
		if context.HasPreviousReview {
			context.LatestPreviousReviewPath = context.PreviousReviewPaths[context.PreviousReviewCount-1]
		}
		if context.HasPreviousRepair {
			context.LatestPreviousRepairPath = context.PreviousRepairPaths[context.PreviousRepairCount-1]
		}
		if len(context.PreviousQAPaths) > 0 {
			context.HasPreviousQA = true
			context.LatestPreviousQAPath = context.PreviousQAPaths[len(context.PreviousQAPaths)-1]
		}
		if context.HasPreviousFixSummary {
			context.LatestPreviousFixSummaryPath = context.PreviousFixSummaryPaths[context.PreviousFixSummaryCount-1]
		}
	}
	if kind == "archive" {
		if usesQualityLoop(state.Workflow) {
			appendQualityLoopArchivePaths(&context, state, runPath)
		} else {
			appendFiniteArchivePaths(&context, state, runPath)
		}
		finalizePreviousArtifactContext(&context)
	}
	if context.LatestPreviousRepairPath != "" {
		context.LatestPreviousRepairFile = filepath.Base(context.LatestPreviousRepairPath)
	}
	if context.LatestPreviousQAPath != "" {
		context.LatestPreviousQAFile = filepath.Base(context.LatestPreviousQAPath)
	}
	return context, nil
}

// appendPreviousQualityArtifacts adds only completed, valid dynamic repair artifacts.
func appendPreviousQualityArtifacts(context *promptTemplateContext, state State, runPath, prefix string, iteration int) error {
	for i := 1; i < iteration; i++ {
		stage := fmt.Sprintf("%s%d", prefix, i)
		if state.Stages[stage] != "completed" {
			continue
		}
		name := fmt.Sprintf("audit-%d.json", i)
		if prefix == "targeted_repair_" {
			name = fmt.Sprintf("targeted-repair-%d.json", i)
		}
		path := filepath.Join(runPath, name)
		if _, err := ReadRepair(path); err != nil {
			return fmt.Errorf("已完成阶段 %s 缺少有效 repair artifact %s: %w", stage, path, err)
		}
		context.PreviousRepairPaths = append(context.PreviousRepairPaths, path)
	}
	context.PreviousRepairCount = len(context.PreviousRepairPaths)
	context.HasPreviousRepair = context.PreviousRepairCount > 0
	if context.HasPreviousRepair {
		context.LatestPreviousRepairPath = context.PreviousRepairPaths[context.PreviousRepairCount-1]
	}
	return nil
}

// latestQualityLoopRepairCheckpoint selects the audit or targeted repair immediately preceding QA.
func latestQualityLoopRepairCheckpoint(state State, runPath string, qaIteration int) (string, error) {
	if qaIteration > 1 {
		stage := fmt.Sprintf("targeted_repair_%d", qaIteration-1)
		if state.Stages[stage] == "completed" {
			path := filepath.Join(runPath, fmt.Sprintf("targeted-repair-%d.json", qaIteration-1))
			if _, err := ReadRepair(path); err != nil {
				return "", fmt.Errorf("已完成阶段 %s 缺少有效 repair artifact %s: %w", stage, path, err)
			}
			return path, nil
		}
	}
	latestAudit := 0
	for stage, status := range state.Stages {
		if status != "completed" || !strings.HasPrefix(stage, "audit_") {
			continue
		}
		parsed, err := parseWorkflowStage(stage)
		if err == nil && parsed.Iteration > latestAudit {
			latestAudit = parsed.Iteration
		}
	}
	if latestAudit > 0 {
		stage := fmt.Sprintf("audit_%d", latestAudit)
		path := filepath.Join(runPath, fmt.Sprintf("audit-%d.json", latestAudit))
		if _, err := ReadRepair(path); err != nil {
			return "", fmt.Errorf("已完成阶段 %s 缺少有效 repair artifact %s: %w", stage, path, err)
		}
		return path, nil
	}
	return "", nil
}

// appendQualityLoopArchivePaths derives every dynamic audit, repair, and QA artifact from stage records.
func appendQualityLoopArchivePaths(context *promptTemplateContext, state State, runPath string) {
	for _, stage := range observedStatusStages(state) {
		if state.Stages[stage] == "" {
			continue
		}
		iteration, err := stageIteration(stage)
		if err != nil {
			continue
		}
		switch {
		case strings.HasPrefix(stage, "audit_"):
			context.PreviousRepairPaths = append(context.PreviousRepairPaths, filepath.Join(runPath, fmt.Sprintf("audit-%d.json", iteration)))
		case strings.HasPrefix(stage, "targeted_repair_"):
			context.PreviousRepairPaths = append(context.PreviousRepairPaths, filepath.Join(runPath, fmt.Sprintf("targeted-repair-%d.json", iteration)))
		case strings.HasPrefix(stage, "qa_"):
			context.PreviousQAPaths = append(context.PreviousQAPaths, filepath.Join(runPath, fmt.Sprintf("qa-%d.json", iteration)))
		}
	}
}

// appendFiniteArchivePaths preserves artifact collection for sealed legacy generations.
func appendFiniteArchivePaths(context *promptTemplateContext, state State, runPath string) {
	iterationLimit := state.Workflow.MaxRepairIterations
	if state.Workflow.MaxReviewIterations > iterationLimit {
		iterationLimit = state.Workflow.MaxReviewIterations
	}
	if usesRepairWorkflow(state.Workflow) && iterationLimit == 0 {
		iterationLimit = 1
	}
	for i := 1; i <= iterationLimit; i++ {
		if state.Stages[fmt.Sprintf("repair_%d", i)] != "" {
			context.PreviousRepairPaths = append(context.PreviousRepairPaths, filepath.Join(runPath, fmt.Sprintf("repair-%d.json", i)))
		}
		if state.Stages[fmt.Sprintf("qa_%d", i)] != "" {
			context.PreviousQAPaths = append(context.PreviousQAPaths, filepath.Join(runPath, fmt.Sprintf("qa-%d.json", i)))
		}
	}
	for i := 1; !usesRepairWorkflow(state.Workflow) && i <= state.Workflow.MaxReviewIterations; i++ {
		reviewStage := fmt.Sprintf("review_%d", i)
		if state.Stages[reviewStage] == "" {
			continue
		}
		context.PreviousReviewPaths = append(context.PreviousReviewPaths, filepath.Join(runPath, fmt.Sprintf("review-%d.json", i)))
		qaStage := fmt.Sprintf("qa_%d", i)
		if state.Stages[qaStage] != "" {
			context.PreviousQAPaths = append(context.PreviousQAPaths, filepath.Join(runPath, fmt.Sprintf("qa-%d.json", i)))
		}
		fixStage := fmt.Sprintf("fix_%d", i)
		if state.Stages[fixStage] != "" {
			context.PreviousFixSummaryPaths = append(context.PreviousFixSummaryPaths, filepath.Join(runPath, fmt.Sprintf("fix-%d-summary.md", i)))
		}
	}
}

// finalizePreviousArtifactContext updates prompt flags after archive-specific collection.
func finalizePreviousArtifactContext(context *promptTemplateContext) {
	context.PreviousReviewCount = len(context.PreviousReviewPaths)
	context.PreviousRepairCount = len(context.PreviousRepairPaths)
	context.PreviousFixSummaryCount = len(context.PreviousFixSummaryPaths)
	context.HasPreviousReview = context.PreviousReviewCount > 0
	context.HasPreviousRepair = context.PreviousRepairCount > 0
	context.HasPreviousQA = len(context.PreviousQAPaths) > 0
	context.HasPreviousFixSummary = context.PreviousFixSummaryCount > 0
	if context.HasPreviousReview {
		context.LatestPreviousReviewPath = context.PreviousReviewPaths[context.PreviousReviewCount-1]
	}
	if context.HasPreviousRepair {
		context.LatestPreviousRepairPath = context.PreviousRepairPaths[context.PreviousRepairCount-1]
	}
	if context.HasPreviousQA {
		context.LatestPreviousQAPath = context.PreviousQAPaths[len(context.PreviousQAPaths)-1]
	}
	if context.HasPreviousFixSummary {
		context.LatestPreviousFixSummaryPath = context.PreviousFixSummaryPaths[context.PreviousFixSummaryCount-1]
	}
}

// qaFindingSummaries renders compact evidence-bearing findings for a targeted prompt.
func qaFindingSummaries(qa QA) []string {
	items := make([]string, 0, len(qa.Findings))
	for _, finding := range qa.Findings {
		items = append(items, fmt.Sprintf("[%s] %s — %s", finding.Severity, finding.Title, finding.Evidence))
	}
	return items
}

// failedAcceptanceIDs extracts only explicitly failed acceptance entries from the latest QA.
func failedAcceptanceIDs(qa QA) []string {
	var ids []string
	for _, result := range qa.AcceptanceMatrix {
		if result.Status != "passed" && strings.TrimSpace(result.ID) != "" {
			ids = append(ids, result.ID)
		}
	}
	return ids
}

// promptRoleSession returns the current stage role's backend-scoped session identity.
func promptRoleSession(state State) (string, string) {
	options, err := state.Workflow.StageOption(state.Stage)
	if err != nil || options.Tool == "" {
		return "", ""
	}
	key := sessionStateKey(options.Tool, stageSessionRoleForState(state, state.Stage))
	return key, state.Sessions[key]
}
