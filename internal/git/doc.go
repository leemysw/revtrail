// Package git 用 git 命令读写仓库：状态、分支、历史、比较、提交（含按块提交）、合并、变基、储藏、工作树与远端同步。
//
// L2 | 父级: internal（L1 见 AGENTS.md）
//
// 成员清单：
//   - blocks.go：BlockSelection、PatchDigest、RollbackBlock：按块提交与回滚。
//   - branches.go：分支、标签的列出、切换、新建、重命名、删除与恢复；MyBranches、UserName。
//   - checks.go：commitIndex.check：提交前检查。
//   - commit_diff.go：CommitDiff：提交将带上的差异。
//   - commit_selection.go：commitIndex、CommitSelected、WorkingFileDiff、ChangeDiff：在临时索引上按选择提交。
//   - git.go：Command / Network / run：统一环境的 git 进程；状态解析与 Change。
//   - history.go：日志（区分标签）、提交详情、图谱、未推送 / 待拉取提交与文件统计（含 --name-status 状态）。
//   - ignore.go：Ignored：被忽略路径的查询。
//   - inspect.go：Blame、FileHistory、CompareRefs、ApplyPatchText、DiffFiles。
//   - log_filter.go：LogFilter（含路径、排序）、FilteredLog、ResolveCommit。
//   - merge.go：ReadMerge、MergeFile.Apply：三方内容与写回。
//   - operations.go：合并、变基、继续 / 跳过 / 中止、Smart Checkout 自动储藏、拣选、还原（含 RevertFiles）。
//   - push.go：PushPlan：待推提交与推送。
//   - rebase.go：RebasePlan、InteractiveRebase、Autosquash。
//   - remote.go：Tracking、Fetch、UpdateSummary、WebURL 等。
//   - repositories.go：Roots：工作区内的 git 仓库根。
//   - reword.go：Reword、CommitMessage、Pushed：不动工作区改写提交信息。
//   - stash.go：储藏的列出（含提交哈希）、文件（含未跟踪文件）、保存、查看、应用、建分支、删除与清空。
//   - worktree.go：Worktrees、AddWorktree、RemoveWorktree：工作树。
//   - *_test.go：行为测试（12 个文件）。
//
// 所有 git 进程都经 git.go 的统一环境启动（无锁、无提示、无分页、英文输出），上层不拼 git 命令。
//
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package git
