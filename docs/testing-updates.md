# 升级链路测试指南（test-v 测试渠道）

更新日期：2026-10-20。状态：macOS arm64 与 Windows amd64 均已完成 `0.0.1-rc.1` → `0.0.1-rc.2` 实机升级；正式 `v0.0.1` 尚未发布。本文只用于验证自动更新，不发布正式版本；发布规则见 [release.md](release.md)，构建与日志位置见 [development.md](development.md)。

> macOS 实测约束：ad-hoc 签名的测试版请安装在 `~/Applications/Lucia.app`。安装在 `/Applications` 时，更新可以下载、验签并写入，但 macOS 27 的 App Management/LaunchServices 可能让重启后的进程卡在 dyld（进程存在、无窗口）。正式版在 Developer ID 签名并公证后再建议放入 `/Applications`。

## 本轮实测记录（2026-10-20）

| 平台 | 安装位置 | 结果 | 关键证据 |
|---|---|---|---|
| macOS arm64 | `~/Applications/Lucia.app` | 通过 | rc.1 → rc.2；下载并校验 12,507 B delta；自动重启；新进程 `version=0.0.1-rc.2`；关于页、设置和窗口位置保留 |
| Windows amd64 | `%LOCALAPPDATA%\Programs\Lucia\Lucia.exe` | 通过 | rc.1 → rc.2；下载并校验 12,833 B delta（与清单一致）；自动重启；新进程 `version=0.0.1-rc.2 commit=a94ddd3dbcc2`；设置和窗口保留 |

Windows 升级前有三次 GitHub API `403`（14:59、15:04、15:09），15:22 重试成功；升级后日志无错误。未出现 SmartScreen 提示；exe 为 `NotSigned`，外部分发仍有潜在提示风险。macOS 的 `/Applications` 限制见下方第 3 节。

## 1. 先明确：用哪条“预发布”通道

MyGo v0.5.5 的更新器只接受**已发布、非草稿、且没有勾选 GitHub “Pre-release”** 的 Release（`internal/update/github.go` 的 `latestTagged`）。因此：

- **不要**用 GitHub Release 的 “Set as a pre-release” 复选框：勾选后应用会直接忽略它。
- 这里的“预发布”体现在**版本号**（`0.0.1-rc.1`）和 **`test-v` 标签前缀**，不是 GitHub Release 的 Pre-release 标记；Release 本身必须是普通的已发布状态。
- **不要**为了测试创建 `v0.0.1` 这类正式标签。
- 使用 `test-v<版本>` 标签：workflow 会把它发布成**普通 Release**，但设置 `--latest=false`，标题和说明写“测试版，勿用”。应用构建内嵌 `tagPrefix: test-v`，只会查找 `test-v*`，这就是项目的“预发布”通道。
- `test-v*` 的版本必须带预发布后缀，例如 `0.0.1-rc.1`；这样测试版本不会占用正式 `0.0.1`。workflow 对 `test-v*` 强制要求该后缀，正式 `v*` 则仍要求纯 `major.minor.patch`。

本次测试使用两个测试版本：

| 角色 | 版本 | 标签 |
|---|---|---|
| 已安装的旧版 | 0.0.1-rc.1 | `test-v0.0.1-rc.1` |
| 待升级的新版 | 0.0.1-rc.2 | `test-v0.0.1-rc.2` |

两个版本可以来自同一个提交，只有标签版本和更新说明不同；这样测试的是真实下载、签名校验、替换和重启链路，不会混入业务改动。

注意：`0.0.1-rc.2` 是 `0.0.1` 的预发布版本，只存在于 `test-v` 测试渠道，**没有占用正式 `0.0.1` 的版本号**。正式首版仍是 `v0.0.1`；测试设备完成演练后卸载测试版并安装正式版即可。

首个正式版发布前，GitHub 的 `releases/latest` 仍可能回退指向最新的测试版；本次测试不受影响，因为测试构建按 `test-v` 前缀查找版本，而不是读 latest。正式渠道的隔离在首个 `v*` 发布后才有完整对象可验证。

## 2. 测试环境与准备

### 需要你准备

- [ ] **确认平台**：macOS arm64、Windows amd64，或两者都测。建议先 macOS，通过后再在 Windows 实机复测。
- [ ] **授权代码操作**：当前工作区有未提交变更；打标签前需要先提交并推送 `main`。你可以自己完成，或明确授权我提交/推送（按项目 Git 规则使用 `eruoos` 提交、`LoTwT` 查询/合并等）。
- [ ] **macOS 测试机**：Apple Silicon，能安装 DMG 到用户可写的 `~/Applications`；测试版**不要**放进 `/Applications`（原因见第 3 节）。不需要 Apple Developer 会员。
- [ ] **Windows 测试机**：x64 Windows 10/11，能运行 NSIS 每用户安装包；不需要代码签名证书。
- [ ] **网络**：能访问 `github.com`、`api.github.com` 和 Release 资产下载地址；代理需要放行这些域名。
- [ ] **关闭旧实例**：测试前退出所有 `Lucia` 与 `Lucia Dev` 进程，避免单实例把启动转交给旧进程。
- [ ] **备份私钥**：`~/.config/lucia/update-keys/mygo-update.key` 只在本机与仓库 secret 中存在；请存入密码管理器，不要发给我或其他任何人。
- [ ] **准备回传材料**：更新页截图、关于页版本、日志文件；Windows 另外记录 SmartScreen 提示与“应用和功能”中的版本。

### 我会准备

- [ ] 确认 `CHANGELOG.md` 同时有 `0.0.1-rc.1`、`0.0.1-rc.2` 和正式 `0.0.1` 的发布小节（两个 RC 已标注为测试版本）。
- [ ] 运行发布前置检查：标签格式、CHANGELOG、`updates.publicKey`、`updates.github`。
- [ ] 提交并推送当前变更（获得授权后），再创建 `test-v0.0.1-rc.1` 和 `test-v0.0.1-rc.2` 标签。
- [ ] 监控 Release workflow，核对草稿资产、更新清单和自检结果。
- [ ] 分析你回传的截图与日志，定位失败阶段。

### 已就绪项

- 已生成 Ed25519 更新密钥对：私钥 `~/.config/lucia/update-keys/mygo-update.key`（权限 600），公钥已写入 `mygo.json`；本地构建使用该私钥产出了签名归档，说明公私钥匹配。
- `MYGO_UPDATER_PRIVATE_KEY` 已写入 `LoTwT/lucia` 的 Actions secrets。可用下面的命令确认名称（不会显示值）：

  ```sh
  gh secret list --repo LoTwT/lucia
  ```

- `mygo.json` 已内置公钥与 `LoTwT/lucia` 更新源。
- Release workflow 会在 `test-v*` 构建中写入 `tagPrefix: test-v`，并断言 `updates=enabled`。
- `0.0.1-rc.1`、`0.0.1-rc.2` 与正式 `0.0.1` 的发布说明已在 [CHANGELOG.md](../CHANGELOG.md)。

测试机和本地开发都不需要再配置私钥；私钥只用于 GitHub Actions 的发布构建。请不要执行 `cat mygo-update.key`，也不要把私钥内容发到对话里。

## 3. 第一阶段：发布并安装 test-v0.0.1-rc.1

执行方：我打标签并观察 workflow；你下载安装。

1. 我确认 `main` 已包含当前提交，然后执行：

   ```sh
   git tag test-v0.0.1-rc.1
   git push origin test-v0.0.1-rc.1
   ```

2. `Release` workflow 应完成：
   - 草稿校验通过；
   - macOS 与 Windows 构建成功；
   - `update-darwin-arm64.json` / `update-windows-amd64.json` 的版本为 `0.0.1-rc.1`；
   - 草稿被发布为普通 Release，标题类似“Lucia 0.0.1-rc.1（测试版，勿用）”，不设为 latest。
3. 你从该 Release 下载并安装对应制品：
   - macOS：`Lucia.0.0.1-rc.1.arm64.dmg`，把 `Lucia.app` 拖入 `~/Applications`（用户主目录下的 Applications，不是系统 `/Applications`）；
   - Windows：`Lucia.Setup.0.0.1-rc.1.amd64.exe`，完成每用户安装。
4. 打开应用，记录基线：
   - “关于”页显示 `v0.0.1-rc.1`；
   - “更新”页点击“检查更新”，此时只有 0.0.1-rc.1 自己，应显示“已是最新版本”；
   - 回传“关于”和“更新”页截图。

### macOS 为什么把测试版放在 `~/Applications`

2026-10-20 在 macOS 27.0.1（26A434, arm64）实测：测试版放在 `/Applications` 时，`0.0.1-rc.1` 能发现并安装 `0.0.1-rc.2`，日志出现 `installed`、`restarting`、`exited`，但新进程启动后卡在 `_dyld_start`，没有窗口，也没有新的 `version=` 日志行。把同一份 `Lucia.app` 放到 `~/Applications` 后，同一升级流程完整通过：下载 12,507 字节的 delta、验签、替换、自动重启，日志出现 `version=0.0.1-rc.2`，窗口与设置保留。

这是系统对 `/Applications` 内 ad-hoc 应用的 App Management/LaunchServices 限制，不是更新清单或签名校验失败。正式分发到 `/Applications` 前，需要 Developer ID 签名并公证；测试阶段用 `~/Applications` 绕过该限制。若已误装到 `/Applications`，退出应用，把应用移到别处，再从 DMG 把 `Lucia.app` 复制到 `~/Applications` 后启动；设置与日志不需要删除。

macOS 日志：`~/Library/Logs/Lucia/lucia.log`。Windows 日志：`%LOCALAPPDATA%\Lucia\logs\lucia.log`。

## 4. 第二阶段：发布 test-v0.0.1-rc.2 并执行升级

执行方：我打标签；你完成点击与观察。

1. 我执行：

   ```sh
   git tag test-v0.0.1-rc.2
   git push origin test-v0.0.1-rc.2
   ```

2. workflow 发布 `0.0.1-rc.2` 后，你在已打开的 0.0.1-rc.1 应用中：
   - 进入“更新”页；
   - 点击“检查更新”（不必等 24 小时自动检查）；
   - 预期显示“发现新版本 0.0.1-rc.2”和 `CHANGELOG` 中的测试说明。
3. 点击“安装并重启”。请记录：
   - 下载是否出现进度；
   - 是否出现错误文本；
   - 应用是否自动退出并重新打开；
   - 重新打开后的“关于”版本。
4. 升级后再次点击“检查更新”，应显示“已是最新版本”。
5. 检查设置与窗口位置是否保留，数据目录是否仍是升级前的位置。

### 预期日志顺序

macOS 示例：

```text
lucia: version=0.0.1-rc.1 commit=... platform=darwin/arm64 data="..." logs="..."
lucia: update check: found 0.0.1-rc.2 (running 0.0.1-rc.1)
lucia: update install: downloading and verifying 0.0.1-rc.2
lucia: update install: downloaded N bytes
lucia: update install: 0.0.1-rc.2 installed
lucia: update installed, restarting
lucia: exited
lucia: version=0.0.1-rc.2 commit=... platform=darwin/arm64 data="..." logs="..."
```

具体行文以 MyGo v0.5.5 的输出为准；关键是能看出“检查到 0.0.1-rc.2 → 下载并验签 → 安装 → 重启 → 新进程版本 0.0.1-rc.2”。

## 5. 可选：负向与边界测试

按时间选择，不必全部执行：

- [ ] **草稿/预发布隔离**：把 0.0.1-rc.2 的 Release 勾成 “Pre-release” 或保留为草稿后，在 0.0.1-rc.1 应用中检查更新，应找不到它。验证后取消勾选并重新发布，恢复可更新状态。不要删除 0.0.1-rc.1 资产。
- [ ] **离线**：断开网络后点击检查，界面应显示检查失败且应用继续可用；恢复网络后重试成功。
- [ ] **重复检查**：连续点击检查，不应并发重复下载；按钮在检查/下载期间禁用。
- [ ] **签名拒绝**：如要做，使用一次性测试密钥或本地篡改归档演练，不要用正式私钥做破坏性操作；预期安装失败并保留旧版本。
- [ ] **渠道隔离**：需要先存在一个正式 `v*` Release 才能完整验证。本次不创建正式版本，因此这一项留到正式首发后。

## 6. 回传模板

请按下面格式发回，截图可以直接附在对话里：

```text
平台：
安装版本（关于页）：
更新目标版本：
检查结果（找到/未找到/报错）：
是否自动重启：
重启后版本：
设置是否保留：
Windows SmartScreen/安装路径：
日志文件：
截图：
备注：
```

日志可以脱敏本机用户名，但请保留版本、时间、`update check`、`update install` 和重启相关行。

## 7. 判定标准

本次测试通过需要同时满足：

- 0.0.1-rc.1 能发现 0.0.1-rc.2，说明来自 CHANGELOG 的 0.0.1-rc.2 小节；
- 下载完成且签名校验通过，没有“not signed with the app's key”；
- 安装后自动重启，新进程版本为 0.0.1-rc.2；
- 数据目录、设置和窗口位置保留；
- 再次检查显示已是最新；
- 日志包含检查、下载、安装、重启各阶段。

只完成构建和发布、没有实机点击升级，不能算升级链路已验证。

## 8. 失败处理

| 现象 | 先查 |
|---|---|
| Release workflow 失败 | Actions 日志；`MYGO_UPDATER_PRIVATE_KEY` 是否存在；CHANGELOG 是否有对应小节；测试标签格式是否 `test-vX.Y.Z-<prerelease>`。 |
| 0.0.1-rc.1 找不到 0.0.1-rc.2 | 0.0.1-rc.2 是否已发布（不是草稿、不是 Pre-release）；标签是否为 `test-v0.0.1-rc.2`；GitHub API 是否限流；`update-darwin-arm64.json` 的版本与 URL。 |
| 清单 404 | 资产是否上传完整；Release 是否已发布；tag 前缀是否被 workflow 正确写入构建。 |
| 下载后验签失败 | 私钥与 `mygo.json` 公钥是否同一对；是否使用了旧密钥构建；资产是否被改动。 |
| 安装失败 | 安装位置是否可写；磁盘空间；macOS 测试版是否放在 `~/Applications`（不要放 `/Applications`）；Windows 是否安装在每用户目录。 |
| 安装后不重启 | 看日志是否出现 `update installed, restarting` 与 `exited`；确认没有旧进程/单实例拦截。 |
| 安装后进程存在但无窗口（macOS） | 日志有 `installed`/`restarting` 但没有新的 `version=`；检查是否装在 `/Applications`。这是 App Management 限制，把应用重新复制到 `~/Applications` 后启动，不要重复点击安装。 |
| 窗口在副屏或看不到 | 检查 `~/Library/Application Support/Lucia/window-state.json` 是否指向当前不可见的显示器；退出应用后移走该文件，再启动会居中显示。 |
| 重复提示新版本 | 新进程的实际版本是否真的 0.0.1-rc.2；清单版本是否高于运行版本；是否安装到了旧副本。 |

## 9. 结束与清理

- 测试完成后，`test-v0.0.1-rc.1` 与 `test-v0.0.1-rc.2` 可以保留在 Releases；它们不会被正式渠道的 latest 选中（首个正式版发布后尤其如此）。
- 不要把测试 Release 改成 Pre-release 后继续用于升级测试；那会让已安装的测试版找不到更新。
- 若需要清理，先确认没有测试设备还依赖这些资产，再删除测试 Release；不要删除正式版资产。
- 本机测试数据保留即可；如需重置，退出应用后删除 `~/Library/Application Support/Lucia`（macOS）或 `%LOCALAPPDATA%\Lucia`（Windows），再重新安装。
