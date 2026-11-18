// Package avatar 按邮箱找提交作者的头像：先 GitHub 头像，再 Gravatar，磁盘缓存一周，避免同一份历史反复请求。
//
// L2 | 父级: internal（L1 见 AGENTS.md）
//
// 成员清单：
//   - avatar.go：URLs、Fetch：GitHub / Gravatar 头像，磁盘缓存一周。
//   - *_test.go：行为测试（1 个文件）。
//
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package avatar
