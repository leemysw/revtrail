// Package tools 安装与移除 Revtrail 自己托管的外部工具（ripgrep、gopls），放在本机数据目录的 tools 下，从不写工作区。查找顺序（REVTRAIL_RG / REVTRAIL_GOPLS → 托管副本 → PATH）由 search 与 languages 负责，这里只给出托管副本的路径。
//
// L2 | 父级: internal（L1 见 AGENTS.md）
//
// 成员清单：
//   - tools.go：版本号、InstallTimeout、Remove，以及下载、限长、sha256 校验与路径等共用部分。
//   - ripgrep.go：RipgrepDir / RipgrepPath / RipgrepExists、InstallRipgrep：从 GitHub Release 下载、校验并解出 rg。
//   - gopls.go：GoplsDir / GoplsPath / GoplsExists、GoplsInstallCommand、InstallGopls：用 Go 工具链构建 gopls。
//   - *_test.go：行为测试（1 个文件）。
//
// 只依赖 appdir 与 shellpath，不知道 App；安装都先写暂存目录、成功后再整体改名，失败不留半成品。
//
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package tools
