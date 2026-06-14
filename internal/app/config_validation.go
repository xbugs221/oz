// Package app parses deterministic workflow validation command configuration.
package app

import (
	"fmt"
	"strings"
)

// validationConfigFromInput validates user-supplied quality gate commands.
func validationConfigFromInput(input validationConfigInput, base ValidationConfig) (ValidationConfig, error) {
	config := ValidationConfig{Commands: cloneValidationCommands(base.Commands), MaxAttemptsPerStage: base.MaxAttemptsPerStage}
	if input.Commands != nil {
		config.Commands = cloneValidationCommands(input.Commands)
	}
	for i, command := range config.Commands {
		if strings.TrimSpace(command.Executable) == "" {
			return ValidationConfig{}, fmt.Errorf("validation.commands[%d].executable 不能为空", i)
		}
	}
	if input.MaxAttemptsPerStage != nil {
		return ValidationConfig{}, fmt.Errorf("validation.max_attempts_per_stage 是旧字段，已删除；请使用 validation.limit")
	}
	if input.Limit != nil {
		if *input.Limit < 1 {
			return ValidationConfig{}, fmt.Errorf("validation.limit 必须是正数")
		}
		config.MaxAttemptsPerStage = *input.Limit
	}
	normalizeValidationConfig(&config)
	return config, nil
}

func cloneValidationCommands(commands []ValidationCommand) []ValidationCommand {
	cloned := make([]ValidationCommand, 0, len(commands))
	for _, command := range commands {
		cloned = append(cloned, ValidationCommand{Executable: command.Executable, Args: append([]string(nil), command.Args...)})
	}
	return cloned
}
