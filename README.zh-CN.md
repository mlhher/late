<h1 align="center">Late</h1>

<p align="center">
  <a href="README.md">English</a> | <a href="README.zh-CN.md">简体中文</a>
</p>

<p align="center">
  <b>不要再让模型的推理能力继续退化。</b><br><br>
  一个极简、零配置的 AI 编程智能体。<br>
  强制使用短暂的子智能体来保留模型的智能，并保持上下文纯净。<br>
  从微型的本地模型到 Sol, Fable, 以及 Kimi K3 均可支持。<br>
  用任何 LLM 完成真正的工作。
</p>

<p align="center">
  <a href="https://github.com/mlhher/homebrew-late"><img src="https://img.shields.io/badge/Homebrew-tap-blue.svg?style=for-the-badge" alt="Homebrew"></a>
  <a href="https://github.com/mlhher/late-cli/releases"><img src="https://img.shields.io/github/v/release/mlhher/late-cli?style=for-the-badge&color=green" alt="Release"></a>
  <a href="https://deepwiki.com/mlhher/late-cli"><img src="https://img.shields.io/badge/DeepWiki-docs-blue.svg?style=for-the-badge" alt="DeepWiki"></a>
</p>

> [在本地 LLM 工作流中超越 Claude Code 和 Codex](https://agentnativedev.medium.com/outperforming-claude-code-and-codex-for-local-llm-workflows-5de0e2b1add5) — Agent Native
>
> *"Late-CLI 简直令人惊叹…… 我震惊于它的 Token 消耗竟如此之低，我总觉得自己会收到 DeepSeek 的天价账单。"* — GitHub Discussions
>
> *"同样的模型，在 Late 里感觉更聪明了。"* — Reddit
>
> **使用 Late 构建:** Late 的开发主要在 Late 自身内完成。

<div align="center">
  <br/>
  <img src="assets/late-subagent-handoff.png" alt="Late Orchestrator planning a multi-phase implementation and spawning the first subagent">
  <br/>
  <i>Late 主控节点正在制定计划，并生成原子级子智能体进行精准编辑。</i>
  <br/><br/>
</div>

## 10 秒快速开始

单一的静态编译二进制文件。零依赖。不需要 Python 虚拟环境，不需要 NodeJS。

```bash
# Linux / macOS (Homebrew)
brew tap mlhher/late && brew install late

# 通用备用方案 (Linux / macOS / Windows WSL)
curl -sfL https://raw.githubusercontent.com/mlhher/late-cli/main/install.sh | bash

# 在任何项目中即刻运行
cd your-project
late
```

*(Arch Linux: `yay -S late-cli-bin` | [手动下载二进制文件 (包含原生 Windows)](https://github.com/mlhher/late-cli/releases))*

## 架构瓶颈

**问题所在：** 标准的编程智能体试图在单一共享的上下文窗口内完成所有操作。每一次编译错误、代码检查失败，甚至是文件写入，都会在 KV 缓存中不断堆积。随着上下文充斥着垃圾信息，模型的推理能力会严重退化。你可能会责怪模型，但这实际上是架构的失败。

**Late 的解决方案：** Late 将大脑一分为二。它在“规划”与“执行”之间强制划定严格界限，并主动对智能体的身份和目标进行隔离。

```text
                              [ 用户提示词 ]
                                    │
                                    ▼
┌──────────────────────────────────────────────────────────────────┐
│    主控节点 MAIN ORCHESTRATOR (~1,000 Token 系统提示词)          │
│ 始终处于规划状态。分析意图，梳理架构，制定总计划。               │
│ 上下文窗口保持 100% 纯净（仅保留有效信号）。                     │
└──────┬────────────────────────────────────────────────────┬──────┘
       │ (生成)                                             │ (生成)
       ▼                                                    ▼
┌────────────────────────────────┐           ┌────────────────────────────────┐
│ 临时子智能体：编程 (Coding)    │           │ 临时子智能体：研究 (Research)  │
│ - 执行精准匹配的代码差异修改   │           │ - 抓取并汇总数据               │
│ - 承担检查/写入/重试带来的噪音 │           │ - 承担原始数据带来的上下文膨胀 │
└──────────────┬─────────────────┘           └────────────────┬───────────────┘
               │                                              │
               ▼                                              ▼
        [ 上下文销毁 ]                                 [ 上下文销毁 ]
               │                                              │
               └─────────────► [ 返回执行结果 ] ◄────────────┘
                                      │
                                      ▼
                        ( 🔄 主控节点继续规划和执行 )
```

主控节点的上下文只会因为真正重要的信息而增长：也就是你明确的指令和确定的结果。子智能体为了完成任务所做的一切中间过程，都会从记忆中被彻底抹去。**同样的模型在 Late 中让人感觉更聪明，因为它纯粹基于“信号”而非“噪音”进行推理。**

## 特性矩阵

|  | Late | Claude Code | OpenCode | The Weekly Clone |
| --- | --- | --- | --- | --- |
| **工作流** | **自主编排** | 手动切换 | 手动切换 | 盲目执行 / 手动切换 |
| **代码实现** | **临时编程子智能体 (自动抹除)** | 充斥主上下文 | 充斥主上下文 | 充斥主上下文 |
| **探索** | **临时研究子智能体 (自动抹除)** | 充斥主上下文 | 充斥主上下文 | 充斥主上下文 |
| **KV-Cache** | **严苛的 KV 缓存管理 (无重复的提示词处理)** | 暴力堆砌 | 暴力堆砌 | 暴力堆砌 |
| **系统提示词** | **~1,000 tokens (始终处于规划状态)** | 10,000+ tokens | 10,000+ tokens | ~300-1000+ tokens (无工作流) |
| **依赖** | **零依赖静态二进制文件** | Node.js | Node.js | Python / Node.js |
| **安装要求** | **无 (原生支持 `llama-server`)** | Anthropic OAuth | 强制 JSON 调整 | 无尽的 YAML/TOML/JSON 配置 |
| **设计初衷** | **追求 10 倍效率的开发者** | 企业级报销大户 | 摆弄各种设置 | 追求 GitHub Stars |

## 模型连接

Late 适配任何模型。

**本地模型 (零配置):**
开箱即用。Late 默认指向运行在 `:8080` 端口的 `llama.cpp` (`llama-server` 的默认端口)，无需任何额外配置。

**云端模型 (DeepSeek, Claude, GPT, Kimi, GLM 等):**

```bash
export OPENAI_BASE_URL="你的-API-URL"
export OPENAI_API_KEY="你的-API-密钥"
export OPENAI_MODEL="模型名称"
```

📖 **[阅读快速入门指南](./docs/quickstart.zh-CN.md)** 了解如何持久化保存这些设置，以及 MCP 设置、智能体技能 (Agent Skills)、Git 工作树、快捷键等更多高级功能。

## 更多特性

* **混合模型路由 (Hybrid Model Routing):** 使用强大的推理模型 (例如 GPT 5.6, Kimi K3, GLM 5.2) 制定计划，然后自动生成子智能体，使用极速且廉价的本地模型 (例如 Gemma 4) 来执行具体的代码实现。
* **精准差异对比 (Exact-match Diffs):** 采用严格的 `search`/`replace` 逻辑，并在匹配失败时实现自动自我修复。编辑失败会明确报错。我们绝不默默破坏文件。
* **智能体技能支持 (Agent Skills Support):** 通过使用第三方智能体技能扩展 Late 的能力。无需任何配置。
* **MCP 协议集成 (MCP Integration):** 通过标准 I/O 原生连接外部模型上下文协议 (Model Context Protocol) 服务器。
* **上下文感知搜索 (Context-Aware Search):** 原生搜索工具会自动遵循 `.gitignore` 和 `.llmignore` 规则，防止无关文件淹没上下文窗口。
* **状态持久性 (Stateful Resilience):** 主控节点将连续的会话历史记录保存在磁盘上。即使关闭终端或重启机器，也能从上次中断的地方无缝继续。
* **Git 工作树支持 (Git Worktree Support):** 支持跨分支运行独立并行的智能体实例，而不会出现上下文冲突。
* **人机协作 (Human-in-the-loop):** 读取操作自动批准以保证效率，代码修改强制拦截等待 `[y/N]` 确认。支持会话级、项目级和全局级的信任授权范围，并带有 TTL 自动衰减机制。

## 开源协议与声明

本项目旨在为个人和团队创造工程杠杆，而非为 AI 初创公司提供免费的底层基础设施。

* **对开发者完全免费:** 任何开发者都可以自由使用 Late 来为任何项目（包括商业项目）编写代码。你生成的代码产出完全归属于你。
* **商业基础设施限制:** 你不得对 Late 本身进行商业化变现。将该编排引擎封装成付费服务需要事先获得商业许可协议。*(将于 2030 年 2 月 21 日自动转为 GPLv2 协议)。*
