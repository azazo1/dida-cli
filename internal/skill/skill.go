// Package skill 内嵌面向 agent 的使用指南.
//
// 指南随二进制一起分发, 不依赖任何外部文件, 因此 agent 在任何机器上
// 都可以用 dida skill 拿到与当前版本完全一致的用法说明.
package skill

import _ "embed"

//go:embed skill.md
var Guide string
