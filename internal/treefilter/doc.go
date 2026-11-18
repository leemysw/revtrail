// Package treefilter 构造项目树的筛选结果：路径包含查询中每个词的文件及其所在目录，使树能显示还没展开的目录里的命中。
//
// L2 | 父级: internal（L1 见 AGENTS.md）
//
// 成员清单：
//   - treefilter.go：BuildTree：命中文件及其所在目录构成的树。
//   - *_test.go：行为测试（1 个文件）。
//
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package treefilter
