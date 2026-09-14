# Go 工具链 1.27.1 升级评估探针

日期：2026-09-14。状态：**本地评估探针，未改变任何锁定配置**。正式归属 W02 后续（foundation 资格验证）；go directive 仍为 1.25.0，CI 仍为 GOTOOLCHAIN=local + 1.25.x，bindings 生成仍钉 `GOTOOLCHAIN=go1.25.0`。ledger 记录见 WV3-L134。

## 1. 评估方法

在 Windows 10 22H2 x64、go 1.25.0 本机工具链上，用 `GOTOOLCHAIN=go1.27.1`（自动下载 go1.27.1 windows/amd64，2026-09-01 发布）对 commit `4a641b5e`（Wails v3.0.0-beta.12 对齐后）执行四项检查，全程不修改 go.mod/go.sum/CI。

## 2. 结果

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 全量编译 | `GOTOOLCHAIN=go1.27.1 go build ./...` | 通过（exit 0） |
| 静态检查 | `GOTOOLCHAIN=go1.27.1 go vet ./cmd/netcatty/... ./internal/...` | 通过（exit 0） |
| 竞态检测 | `GOTOOLCHAIN=go1.27.1 go test -race -count=1 ./internal/profile/... ./internal/terminal/dataplane/... ./internal/platform/credentials/...` | 5 包全部 ok |
| Bindings 生成 | `GOTOOLCHAIN=go1.27.1 go run .../wails3@v3.0.0-beta.12 generate bindings -d infrastructure/runtime/wails/bindings ./cmd/netcatty` | **非零 diff**：369 包 / 98 models（1.25.0 为 352 包 / 97 models），5 个 bindings 文件变化 |

评估后已用 `GOTOOLCHAIN=go1.25.0` 重新生成 bindings 并 `git checkout` 恢复，工作树回到提交态（352 包 / 97 models，零 diff）。

## 3. 非 zero diff 的根因

Go 1.27 标准库在 `encoding/json` 下新增 `jsontext` 子包。生成器据此把 `json.RawMessage` 的 TS 投影从 `@typedef {any} RawMessage` 解析为 `jsontext.Value`，并在 `encoding/json/models.js`、`cmd/netcatty/models.js`、`cmd/netcatty/syncservice.js`、`internal/platform/cloudsync/models.js`、`internal/plugin/store/models.js` 五个文件加入 `jsontext` import。这是标准库驱动的类型解析变化，与 Wails beta.12 无关（同一 wails 版本、两种工具链，输出不同）。

## 4. 结论与建议

1. **编译器兼容性成立**：build/vet/race 在 1.27.1 下无一处失败；wails beta.12、cgo 相关包（hotkey、conpty、pty）在 Windows 上构建正常。
2. **暂不锁 1.27.1**：bindings 生成形状依赖工具链所选标准库。升级生成工具链是一次协调切片：重新生成 + TS 编译/lint 验证 `jsontext.Value` 投影 + 三平台 smoke，不能只换 CI 里的一行版本号。
3. **`GOTOOLCHAIN=go1.25.0` 生成钉是承重约束**：它保证 bindings 与已提交树字节一致。任何人在不换 go directive 的情况下用更高工具链跑 generate 都会得到上述 5 文件漂移；`check:wails-versions` 只守 wails 版本组合，不守工具链——工具链与 bindings 的一致性由生成命令里的 GOTOOLCHAIN 钉维护。
4. **重估触发条件**：wails 后续 release 显式声明 go 1.27 支持、或我们需要 1.27-only 的标准库/运行时修复时，按第 4.2 条的协调切片推进并补三平台证据。

## 5. 未覆盖

- macOS/Linux 下的 1.27.1 构建（本机无环境）；CGO 竞态在 unix 上的表现。
- `npm run wails:build` 用 1.27.1 的端到端产出验证（评估只到 build/library 层）。
- go1.26.x 未测：若未来 1.27 受阻，1.26.8（2026-09-01）是下一个候选，需重跑同一组检查。
