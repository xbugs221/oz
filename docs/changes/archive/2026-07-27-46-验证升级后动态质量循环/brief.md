# 验证升级后动态质量循环

## 用户问题

本地 `oz` 已从提案 45 的源码升级，需要用一个全新 sealed run 验证默认工作流不再封存为有限轮次的 `repair-v1`，并确认运行状态、动态自查和独立 QA 能正常推进。

## 交付目标

- 通过真实安装版 `oz flow config` 生成默认配置，并验证不再输出固定修复轮次。
- 发起本提案的新工作流，运行期间核对 sealed state 使用 `quality-loop-v1`，且 worker 心跳和动态阶段正常。

## 非目标

- 不修改提案 45 已归档的实现。
- 不对模型发现问题的质量做性能或准确率评测。

## 验收条目

- 场景：升级后的安装版生成无固定轮次上限的动态质量循环配置。

## 执行入口

执行阶段默认读取本文件、`acceptance.json` 和 `docs/changes/archive/2026-07-27-46-验证升级后动态质量循环/tests/test_installed_quality_loop.sh`；运行证据写入 `test-results/46-installed-quality-loop/`。
