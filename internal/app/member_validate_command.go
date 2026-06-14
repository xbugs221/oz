// Package app validates parallel subagent member artifacts from CLI.
package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// runValidateMemberArtifact checks one subagent member.json against the shared schema.
func runValidateMemberArtifact(args []string, stdout io.Writer) error {
	if !hasFlag(args, "--artifact") {
		return fmt.Errorf("用法：wo validate-member-artifact --artifact <path> --group <group> --member <member> --change <change-name>")
	}
	path, err := requireFlagValue(args, "--artifact")
	if err != nil {
		return err
	}
	group, err := requireFlagValue(args, "--group")
	if err != nil {
		return err
	}
	memberName, err := requireFlagValue(args, "--member")
	if err != nil {
		return err
	}
	changeName, err := requireFlagValue(args, "--change")
	if err != nil {
		return err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("member artifact 路径不能为空")
	}
	member := ParallelMemberConfig{Name: memberName}
	result, err := readNormalizeValidateMemberArtifact(path, group, member, changeName)
	if err != nil {
		return helpfulMemberArtifactError(path, err)
	}
	fmt.Fprintf(stdout, "member artifact 合法: %s (member=%s, status=%s)\n", path, result.Name, result.Status)
	return nil
}

// helpfulMemberArtifactError turns schema failures into prompts a subagent can repair.
func helpfulMemberArtifactError(path string, err error) error {
	message := err.Error()
	if strings.Contains(message, "evidence 必须是字符串数组") {
		return fmt.Errorf("%s: field=evidence expected=array<string> actual=%s 修复建议：把 evidence 改成字符串数组，例如 \"evidence\":[\"运行日志路径或关键断言\"]", message, memberArtifactFieldType(path, "evidence"))
	}
	if strings.Contains(message, "evidence 第 ") {
		return fmt.Errorf("%s: field=evidence expected=array<string> actual=array<non-string> 修复建议：evidence 每一项只能是字符串，不能写对象或数组", message)
	}
	return err
}

// memberArtifactFieldType reports a small JSON type name for actionable CLI errors.
func memberArtifactFieldType(path string, field string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "unknown"
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return "unknown"
	}
	value, ok := raw[field]
	if !ok || value == nil {
		return "missing"
	}
	switch value.(type) {
	case []interface{}:
		return "array"
	case map[string]interface{}:
		return "object"
	case string:
		return "string"
	case bool:
		return "bool"
	case float64:
		return "number"
	default:
		return "unknown"
	}
}
