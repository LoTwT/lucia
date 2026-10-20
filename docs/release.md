# 发布与自动更新

更新日期：2026-10-20。本文维护版本规则、两条发布渠道、密钥管理、打标签与发布步骤、发布后验证；构建与本地命令见 [development.md](development.md)，更新状态机见 [architecture.md](architecture.md#4-更新状态机)。

发布由 `LoTwT/lucia` 的 GitHub Releases 承担，也是 MyGo 内置更新器的下载源（MyGo v0.5.5）。

## 版本号与渠道

- **版本号**：正式版为 `major.minor.patch`，首个正式版本为 `0.0.1`，随发布递增；测试版为 `major.minor.patch-<prerelease>`（如 `0.0.1-rc.1`），只能用于 `test-v*` 标签。版本以**标签**为准：源文件 `mygo.json` 保持开发版本，发布 workflow 在构建前把标签版本与渠道前缀写入检出副本，不提交版本号改动。
- **正式渠道**：标签 `v<版本>`，如 `v0.0.1`。更新源为仓库 latest Release；应用只接收正式渠道更新。
- **测试渠道**：标签 `test-v<版本>-<预发布后缀>`，如 `test-v0.0.1-rc.2`。更新源按 `test-v` 前缀查找 Release；发布为普通 Release，但**不设为 latest**，标题与说明注明测试版。两个渠道由 `updates.tagPrefix` 隔离。测试版本必须是预发布版本，不能使用 `test-v0.0.1` 这种会占用正式版本号的写法。
- **latest 的回退**：仓库还没有正式版时，GitHub 的 `releases/latest` 仍可能回退到最新的测试版；首个正式版必须以 `--latest=true` 发布（workflow 已如此），此后 latest 才稳定指向正式渠道。标签前缀路径本身不会把 `test-v…` 当成正式渠道。
- **更新说明**：每个版本必须在 [CHANGELOG.md](../CHANGELOG.md) 有 `## <版本号>` 小节。该节文字进入更新清单的 `notes`，显示在应用“更新”页。缺失小节时草稿 job 在构建前失败。

## 测试渠道升级演练

不要为了验证自动更新创建正式 `v*` 标签。使用 `test-v0.0.1-rc.1` → `test-v0.0.1-rc.2` 两个测试版本即可，workflow 会把它们发布为不设 latest 的普通 Release。两个 RC 都是 `0.0.1` 的预发布版本，不占用正式 `v0.0.1`；测试设备在演练结束后重新安装正式版。

**不要勾选 GitHub 的 “Pre-release”**：MyGo v0.5.5 的更新器只接受已发布、非草稿、非 GitHub 预发布的 Release，勾选后应用会忽略该版本。这里的“预发布”由 `test-v` 标签前缀隔离，而不是 GitHub 的预发布标记。

双方需要准备的内容、分阶段操作、回传材料与判定标准见 [testing-updates.md](testing-updates.md)。

2026-10-20 的 `0.0.1-rc.1` → `0.0.1-rc.2` 演练已在 macOS arm64（`~/Applications`）与 Windows amd64（每用户安装目录）通过；macOS `/Applications` 的 App Management 限制见测试指南。

源文件中的 `mygo.json` 只有公钥与仓库，没有 `tagPrefix`；发布时为标签写入 `v` 或 `test-v`。

## 更新配置与密钥

```json
"updates": {
  "publicKey": "Q85i2CaTao+ak1bn9nH3fxmhH6HnivFG8s8GuHWA4VM=",
  "github": "LoTwT/lucia"
}
```

- 公钥与私钥为 Ed25519 对，由 `go tool mygo keygen` 生成。私钥在本机 `~/.config/lucia/update-keys/mygo-update.key`（权限 600），只应保存在用户密码管理器与仓库 secret `MYGO_UPDATER_PRIVATE_KEY`。
- **私钥绝不进入仓库**。丢失私钥会使已安装应用无法再收到更新；泄露则可向用户推送任意代码。
- 发布构建通过 `MYGO_UPDATER_PRIVATE_KEY` 签名。私钥缺失或与公钥不匹配时，构建不产出更新档案，workflow 的资产核对会失败，不会发布一个“没有更新的版本”。
- 公钥自 `0.0.1` 随包内置。更换密钥需要用户手动重装，不能作为普通更新推送。

首次发布前设置 secret：

```sh
gh secret set MYGO_UPDATER_PRIVATE_KEY < ~/.config/lucia/update-keys/mygo-update.key
```

## 发布步骤

1. 确认目标版本号已写入 [CHANGELOG.md](../CHANGELOG.md)，版本相对上一发布版递增，且要发布的提交已合并 `main`。
2. 在合并提交上打标签并推送：
   - 正式版：`git tag v0.0.1 && git push origin v0.0.1`
   - 测试版：`git tag test-v0.0.1-rc.2 && git push origin test-v0.0.1-rc.2`
3. [Release workflow](../.github/workflows/release.yml) 自动执行：
   1. **draft-release**：校验标签格式、版本号、CHANGELOG 小节与 `updates` 配置，创建草稿 Release。
   2. **macos / windows**：写入版本与渠道前缀 → `go tool mygo build -platform … -upload`（私钥经 secret 注入）→ 校验应用包、安装包、更新归档与清单 → 在打包产物上运行 `LUCIA_SELFTEST` 并断言 `updates=enabled`。
   3. **publish**：两个平台都成功后核对资产齐全、清单版本与标签一致，再发布草稿。测试渠道 `--latest=false` 并加测试版警示；正式渠道 `--latest=true`。
4. 失败处理：单平台失败可在 Actions 中 “Re-run failed jobs”，重跑会覆盖同一草稿中的同名资产。标签格式或 CHANGELOG 问题需修正后重新打标签触发。

## 产物

| 文件 | 说明 |
|---|---|
| `Lucia <版本> arm64.dmg` | macOS 安装镜像；GitHub 存储名把空格转为 `.`，即 `Lucia.<版本>.arm64.dmg`。 |
| `Lucia Setup <版本> amd64.exe` | Windows NSIS 每用户安装包；存储名 `Lucia.Setup.<版本>.amd64.exe`。 |
| `lucia-<版本>-darwin-arm64.tar.gz` | macOS 更新归档（签名）。 |
| `lucia-<版本>-windows-amd64.tar.gz` | Windows 更新归档（签名）。 |
| `update-darwin-arm64.json` | macOS 更新清单。 |
| `update-windows-amd64.json` | Windows 更新清单。 |
| `lucia-<旧版本>-to-<版本>-<target>.delta` | 差量更新，可选；首版没有。 |

清单包含版本、说明、日期与归档/差量的 URL、大小和签名。清单最后上传，应用不会看到指向缺失文件的清单。

## 本地带签名构建

```sh
MYGO_UPDATER_PRIVATE_KEY="$(cat ~/.config/lucia/update-keys/mygo-update.key)" \
  go tool mygo build -platform darwin/arm64
```

产物同时生成签名更新归档与 `update-darwin-arm64.json`。不带私钥的本地构建仍嵌入更新源，但不产出更新档案，构建日志会警告。需要完全关闭更新的测试构建时，从检出副本的 `mygo.json` 移除 `updates` 再构建；`LUCIA_SELFTEST` 应报告 `updates=disabled`。

## 发布后验证

按顺序核对并留证：

1. **资产齐全**：上表必需项都存在；两个 `update-*.json` 的 `version` 与标签一致，归档 `url` 指向本标签的 Release。
2. **构建状态**：从 Release 下载安装包安装后运行自检。测试版 macOS 应用安装到 `~/Applications`（ad-hoc 签名；放到 `/Applications` 可能触发 App Management，原地更新后无法启动）；正式签名版才建议放 `/Applications`：

   ```sh
   LUCIA_SELFTEST=1 ~/Applications/Lucia.app/Contents/MacOS/Lucia
   ```

   Windows 用 PowerShell 将 stderr 重定向到文件。输出必须包含 `updates=enabled` 与标签版本。
3. **检查更新**：在安装旧版的机器上启动，更新页应在启动检查后显示新版本号与 CHANGELOG 说明；手动“检查更新”结果一致。日志应出现检查阶段行。
4. **安装与重启**：点击“安装并重启”后完成下载、签名校验、替换与自动重启；重启后的“关于”页版本与新标签一致，设置与窗口位置保留。
5. **签名拒绝**：用一次性测试密钥或篡改过的归档演练，安装必须失败并保留原版本；错误可见，应用继续运行。不要用正式私钥做破坏性演练。
6. **渠道隔离**：安装测试渠道构建的设备只收到 `test-v` 更新；正式渠道只收到 `v` 更新。测试渠道发布不得成为 latest。
7. **Gatekeeper / SmartScreen**：ad-hoc 签名的 macOS 应用与未签名 Windows 安装包在干净机器上的首次打开提示按实际记录；这不阻止功能验收，但必须在发布说明中如实告知。macOS 测试版必须装在 `~/Applications`：2026-10-20 在 macOS 27 实测，`/Applications` 内的 ad-hoc 应用可以完成更新写入，但重启后的进程会卡在 dyld；正式版需要 Developer ID 签名与公证后再支持 `/Applications` 自更新。
8. **差量更新**：第二个版本起，清单包含差量时验证“优先差量、失败回退整包”；首版没有差量属正常。

未完成上述实机验证前，只能声称工程链路已接通，不能声称某次发布已经完成升级验收。
