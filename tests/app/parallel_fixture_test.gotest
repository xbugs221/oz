// Package app provides parallel artifact fixtures for state-machine tests.
package app

import (
	"fmt"
	"strings"
)

type parallelMemberFixture struct {
	name    string
	purpose string
	status  string
	summary string
	extra   string
}

func parallelReviewArtifactForTest(overrides ...parallelMemberFixture) string {
	return parallelArtifactForTest("review", "gate_input", "review helpers reported", reviewMembersForTest(overrides...))
}

func parallelPlanningArtifactForTest(overrides ...parallelMemberFixture) string {
	return parallelArtifactForTest("planning_context", "advisory", "planning context reported", planningMembersForTest(overrides...))
}

func parallelQAArtifactForTest(overrides ...parallelMemberFixture) string {
	return parallelArtifactForTest("qa", "gate_input", "qa helpers reported", qaMembersForTest(overrides...))
}

func parallelContextArtifactForTest(overrides ...parallelMemberFixture) string {
	return parallelArtifactForTest("implementation_context", "advisory", "implementation context reported", contextMembersForTest(overrides...))
}

func reviewMembersForTest(overrides ...parallelMemberFixture) []parallelMemberFixture {
	return overrideMembers([]parallelMemberFixture{
		successMember("目标核对审核员", "核对 proposal/spec/task 是否满足"),
		successMember("代码质量审核员", "检查类型、边界和可维护性"),
		successMember("测试有效性审核员", "判断测试是否真实覆盖场景"),
		successMember("安全风险审核员", "检查权限、输入、泄漏和破坏性操作"),
		successMember("上下文一致性审核员", "检查是否违背现有架构约定"),
	}, overrides)
}

func planningMembersForTest(overrides ...parallelMemberFixture) []parallelMemberFixture {
	return overrideMembers([]parallelMemberFixture{
		successMember("需求分析员", "找出需求歧义、风险和遗漏"),
		successMember("代码库侦察员", "搜索现有模块、测试入口和实现约定"),
		successMember("外部资料研究员", "查询外部库文档和开源实现"),
	}, overrides)
}

func qaMembersForTest(overrides ...parallelMemberFixture) []parallelMemberFixture {
	return overrideMembers([]parallelMemberFixture{
		successMember("CLI/API 测试员", "执行命令行或接口真实路径"),
		successMember("浏览器路径测试员", "执行页面真实用户路径"),
		successMember("证据采集员", "采集截图、trace、console、network 或 runtime log"),
		successMember("回归场景测试员", "覆盖邻近功能回归"),
	}, overrides)
}

func contextMembersForTest(overrides ...parallelMemberFixture) []parallelMemberFixture {
	return overrideMembers([]parallelMemberFixture{
		successMember("代码库侦察员", "汇总 execution 需要读取的文件和测试模式"),
		successMember("外部资料研究员", "查询 execution 依赖的外部库文档和开源实现"),
	}, overrides)
}

func overrideMembers(defaults []parallelMemberFixture, overrides []parallelMemberFixture) []parallelMemberFixture {
	members := append([]parallelMemberFixture(nil), defaults...)
	for _, override := range overrides {
		for i, member := range members {
			if member.name == override.name {
				members[i] = override
				break
			}
		}
	}
	return members
}

func successMember(name, purpose string) parallelMemberFixture {
	return parallelMemberFixture{name: name, purpose: purpose, status: "success", summary: "passed"}
}

func parallelArtifactForTest(group, mode, summary string, members []parallelMemberFixture) string {
	parts := make([]string, 0, len(members))
	for _, member := range members {
		parts = append(parts, parallelMemberForTest(member))
	}
	return fmt.Sprintf(`{"group":%q,"mode":%q,"summary":%q,"members":[%s]}`, group, mode, summary, strings.Join(parts, ","))
}

func parallelMemberForTest(member parallelMemberFixture) string {
	extra := ""
	if strings.TrimSpace(member.extra) != "" {
		extra = "," + member.extra
	}
	return fmt.Sprintf(`{"name":%q,"purpose":%q,"status":%q,"summary":%q%s}`, member.name, member.purpose, member.status, member.summary, extra)
}
