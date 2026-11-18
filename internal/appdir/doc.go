// Package appdir 给出 Revtrail 的本机数据目录（配置、缓存），统一放在系统目录下的 Revtrail 子目录。
//
// L2 | 父级: internal（L1 见 AGENTS.md）
//
// 成员清单：
//   - appdir.go：ConfigPath、CachePath：Revtrail 目录下的路径。
//
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package appdir
