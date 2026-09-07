# Electron 运行时迁移基线

状态：P0-01 部分完成，`needs-verification`

记录日期：2026-08-23

最新 fixture 刷新：2026-08-31

## 1. 目的

本基线冻结 Wails/Go 迁移需要比较的 Electron 契约、owner、流控常量和
本机性能观测。它不是 Electron 质量验收报告，也不代表 Phase 0 已通过。

后续 Go differential tests 应使用 `testdata/migration/electron/` 中的合成
fixtures，而不是重新从文档描述推断当前行为。

## 2. 源状态

- Git HEAD：`180470a03b83eaef012ce8d1e3c602d40e4f0380`
- 分支：`main`
- fixture 来源：执行时的当前工作树
- fixture 输入文件相对 HEAD 的状态：包含 P0-01 follow-up 的 shared terminal workload、
  exporter 和 benchmark wiring 变更；manifest 固定这些当前工作树输入
- 仓库整体状态：存在大量与本切片无关的预先未提交改动

`testdata/migration/electron/manifest.json` 对 bounded source set 中每个输入文件和
输出 fixture 保存
SHA-256。后续如果源契约变化，`npm run check:migration-electron-baseline` 必须失败，
直到执行者判断该变化是否应更新迁移基线。

## 3. 可重复生成的契约基线

生成命令：

```bash
npm run generate:migration-electron-baseline
npm run check:migration-electron-baseline
node --test scripts/migration/export-electron-contract-fixtures.test.mjs
```

输出：

| Fixture | 内容 |
| --- | --- |
| `capability-catalog.json` | 完整 capability identity、policy 和 surfaces |
| `mcp-tool-specs.json` | MCP tool 名称、RPC、输入和 policy 投影 |
| `cli-capabilities.json` | CLI command、RPC 和 policy 投影 |
| `agent-tool-specs.json` | sidebar/global Agent tool 投影 |
| `bridge-contract-index.json` | 8 个 `NetcattyBridge` 及其相对 TypeScript import 闭包的完整声明文本、SHA 和方法索引 |
| `plugin-contract-baseline.json` | plugin schema ID 和结构/RPC/stream limits |
| `plugin-contract-fixtures.json` | 生产 validator 验证的 manifest/RPC/拒绝路径样例 |
| `terminal-flow-constants.json` | 当前 terminal queue、credit 和 chunk constants |
| `terminal-sustained-output-workload.json` | 当前 Electron sustained benchmark 的确定性 generator 参数、chunk-size distribution、总字节与 payload SHA-256 |
| `runtime-contract-fixtures.json` | 合成 AgentEvent 和 session restore 示例 |
| `runtime-contract-fixtures.typecheck.ts` | 通过 `satisfies AgentEvent[]` 锁定事件 union |
| `runtime-owner-index.json` | 高风险 runtime 区域的 owner 和测试索引 |
| `storage-key-candidates.json` | `storageKeys.ts` 中全部 key/alias 分类候选 |
| `manifest.json` | 输入/输出 SHA-256 与 fixture format version |

安全边界：

- exporter 不读取 `localStorage`、Electron `userData`、真实 session 或环境凭据；
- runtime fixtures 只使用 `example.invalid` 和固定 synthetic IDs；
- 测试拒绝 password/private-key/passphrase/API-key 字段和已知 secret markers；
- owner index 测试验证每个记录路径真实存在；
- session restore 通过 production sanitizer；plugin manifest/RPC/path samples 通过
  production contract validators。

当前结果：14 个文件可确定性生成，9 项 fixture 测试和 AgentEvent TypeScript
assignability check 通过。manifest 哈希完整
bounded source set，包括 capability catalog/adapters/codegen、bridge declarations 及
其 imported contract types、
storage keys、Agent events、session restore、plugin validator/canonical/generated schema
和 terminal flow/workload。文本输入在 hash 前统一为 LF，生成物由
`.gitattributes` 固定为 LF；排序使用 Unicode code-point comparison，不依赖主机
locale。

## 4. Benchmark 协议

### 4.1 环境记录

每次正式基线运行必须记录：

- OS 名称、版本和补丁；
- CPU 型号、逻辑核心数、内存；
- CPU architecture；
- Node、Electron、xterm、WebView/renderer 版本；
- GPU 与 renderer 类型；
- 电源模式、是否在 VM/远程桌面中运行；
- benchmark commit、工作树差异和环境变量；
- 是否显示窗口及窗口是否可见/聚焦。

### 4.2 样本与统计

正式 parity envelope 使用：

- 每种 workload 至少 3 个 warm-up rounds；
- 每种 workload 至少 30 个 measurement rounds；
- 记录 p50、p95、p99、median、maximum 和样本原始 JSON；
- terminal chunks 使用固定 seed、数量、大小分布和总字节；
- 多 session workload 固定为 1、4、8 个并发 session；
- RSS 在开始、steady state、peak 和 settle 后采样；
- 同一 target 的 Electron 与 Wails 运行在同一硬件、电源模式和窗口状态；
- 单次异常不手工删除；只能按预先记录的环境失败规则排除。

正式 target matrix 由 P0-01A 冻结。在该矩阵完成前，本文件中的 Windows 数据只
是 C 级本机观测，不是三平台 release baseline。

### 4.3 Workloads

最低 workload 集合：

1. 10k-line 初始 xterm write 和 keyword rebuild；
2. 1600 chunks、约 9.26 MiB 的 sustained output；
3. long unbroken lines；
4. millions of short lines；
5. hidden/stalled/reloaded renderer；
6. 1/4/8 concurrent sessions；
7. output flood 中的 Ctrl-C/urgent input；
8. terminal route rebind；
9. metadata-only plugin ingress；
10. remote SSH TUI flow pause/ack/drain。

### 4.4 接受原则

- 零 byte loss、duplication、reordering；
- queue/RSS 有界；
- stale generation 不影响新 session；
- urgent input 不被 output starvation；
- Wails 指标不得差于冻结的 Electron acceptance envelope，除非有新的
  `decisions.md` decision；
- throughput smoke 不能替代 repaint、remote SSH 或多 session evidence。

## 5. 当前 Windows 本机观测

环境：

```json
{
  "platform": "win32",
  "arch": "x64",
  "osRelease": "10.0.19045",
  "osVersion": "Windows 10 Pro for Workstations",
  "cpuModel": "11th Gen Intel(R) Core(TM) i7-11800H @ 2.30GHz",
  "cpuCount": 16,
  "totalMemoryGiB": 63.8,
  "node": "v24.14.1",
  "electron": "42.3.3",
  "xterm": "6.1.0-beta.292"
}
```

### 5.1 Keyword highlight responsiveness

命令：

```bash
npm run test:xterm-keyword-highlight-performance
```

结果：通过核心 assertions。

```json
{
  "renderer": "webgl",
  "rawChars": 518193,
  "initialWriteMs": 141,
  "enterWriteMs": 1.8,
  "rebuildMs": 149.2,
  "blueMatchCount": 30000
}
```

残余：退出清理 Netcatty temp 子目录时发生 Windows `EPERM`，但进程正常返回。

### 5.2 Sustained throughput-only smoke

命令：

```powershell
$env:NETCATTY_TERMINAL_PERF_ROUNDS='3'
$env:NETCATTY_TERMINAL_PERF_SUSTAINED_ONLY='1'
npm run test:xterm-keyword-highlight-throughput
```

结果：3 rounds 通过当前 throughput/latency assertions。

| Metric | Current/new median |
| --- | ---: |
| total chars | 9,708,106 |
| chunks | 1,600 |
| stream time | 8,784.7 ms |
| throughput | 1.0539 MiB/s |
| callback p50 | 5.1 ms |
| callback p95 | 6.1 ms |
| callback p99 | 6.6 ms |
| max heartbeat | 21.2 ms |

该模式跳过 quiet-period rebuild/repaint，不得用于宣称完整 benchmark 通过。

### 5.3 完整 throughput/repaint

命令：

```powershell
$env:NETCATTY_TERMINAL_PERF_ROUNDS='3'
npm run test:xterm-keyword-highlight-throughput
```

结果：失败。三轮 current/new 均出现 `paintTimedOut: true`，触发
`quiet catch-up must repaint atomically` assertion。核心 sustained stream 指标保持在
约 1.04-1.06 MiB/s、callback p99 约 6.4-6.6 ms，但完整 repaint gate 未通过。

该失败是当前 Electron 基线证据的一部分。P0-01 不修改 benchmark 规避它。

### 5.4 WebGL atlas overflow

命令：

```bash
npm run test:xterm-webgl-overflow
```

测试输出了成功 marker：

```text
XTERM_WEBGL_ATLAS_OVERFLOW_OK
```

随后 temp directory cleanup 发生 `EPERM`，产生 unhandled rejection，进程未退出并
在 180 秒被工具终止。因此该命令不能登记为通过。

### 5.5 Remote SSH stress

命令：

```bash
node --test --import tsx scripts/terminal-output-stall.live.test.cjs
```

结果：安全跳过，因为未设置 `NETCATTY_TERMINAL_STRESS_TARGETS`。没有读取或创建
真实 SSH 凭据。P0-01 仍缺少 remote flow pause/ack/input evidence。

## 6. Capability/CLI/MCP 回归

命令：

```bash
node --test electron/capabilities/catalog/integrity.test.cjs \
  electron/capabilities/codegen/toolSurfaces.test.cjs \
  electron/capabilities/adapters/cliAdapter.test.cjs \
  scripts/generate-capability-tools.test.cjs
```

结果：42 tests passed，0 failed。

## 7. 环境阻断

`npm ci` 下载依赖并执行 postinstall，但 Electron ABI 的 `node-pty` rebuild 失败：

```text
MSB8040: 此项目需要缓解了 Spectre 漏洞的库
```

这表示当前 Visual Studio Build Tools 缺少对应 x64 Spectre-mitigated libraries。
因此本机不能提供可信的完整 PTY/app build baseline。不要通过跳过 postinstall 或
删除 Spectre 设置来伪造成功；应安装匹配工具链后重新运行。

`npm ci` 未修改 `package.json` 或 `package-lock.json`，但创建了被 Git 忽略的
`node_modules`。

## 8. 未覆盖证据与停止状态

未覆盖：

- macOS Electron/xterm/PTY baseline；
- Linux Electron/xterm/PTY baseline；
- Windows 完整 repaint gate；
- Windows WebGL overflow 正常进程退出；
- Windows native `node-pty` rebuild 和完整 app smoke；
- 30-round 正式统计；
- remote SSH TUI、multi-session、route rebind 和 urgent input workload。

停止状态：`needs-verification`。

P0-01 的 exporter、bridge/storage/plugin/runtime fixtures、owner index、协议和本机
观测已落地，但三平台正式
基线尚未满足。Phase 0 exit 仍然关闭；不得因此启动 production Go runtime。
