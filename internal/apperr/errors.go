// Package apperr 定义全项目共用的错误类型与退出码映射.
//
// 设计目标只有一个: agent 必须能从退出码和 JSON 错误信封里稳定判断
// 到底发生了什么, 而不需要去正则匹配人类可读的错误文本.
package apperr

import (
	"errors"
	"fmt"
)

// Kind 是错误分类, 同时决定进程退出码与 JSON 信封里的 error.type.
type Kind string

const (
	// KindGeneral 表示未归类的通用错误, 退出码 1.
	KindGeneral Kind = "general"
	// KindUsage 表示命令行用法错误, 退出码 2.
	KindUsage Kind = "usage"
	// KindAuth 表示口令缺失或无效, 退出码 3.
	KindAuth Kind = "auth"
	// KindNotFound 表示请求的资源不存在, 退出码 4.
	KindNotFound Kind = "not_found"
	// KindUpstream 表示上游 MCP 服务返回了错误, 退出码 5.
	KindUpstream Kind = "upstream"
	// KindConfirm 表示破坏性操作缺少确认参数, 退出码 6.
	KindConfirm Kind = "confirmation_required"
	// KindReadOnly 表示只读模式拒绝了写操作, 退出码 7.
	KindReadOnly Kind = "read_only"
	// KindNonDestructive 表示非破坏性模式拒绝了破坏性操作, 退出码 7.
	KindNonDestructive Kind = "non_destructive"
)

// ExitCode 把错误分类映射为进程退出码.
//
// 只读模式与非破坏性模式都属于"被本地策略拦下", 共用退出码 7,
// agent 需要区分具体是哪个闸门时读 error.type.
func (k Kind) ExitCode() int {
	switch k {
	case KindUsage:
		return 2
	case KindAuth:
		return 3
	case KindNotFound:
		return 4
	case KindUpstream:
		return 5
	case KindConfirm:
		return 6
	case KindReadOnly, KindNonDestructive:
		return 7
	default:
		return 1
	}
}

// Error 是带分类与修复提示的错误.
//
// Hint 面向 agent, 应当写成一条可以直接执行或直接照做的建议,
// 而不是一句安抚性的话.
type Error struct {
	Kind    Kind
	Message string
	Hint    string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Cause }

// New 构造一个带分类的错误.
func New(kind Kind, message string) *Error {
	return &Error{Kind: kind, Message: message}
}

// Newf 构造一个带分类的格式化错误.
func Newf(kind Kind, format string, args ...any) *Error {
	return &Error{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

// Wrap 在保留错误分类的前提下附加底层原因.
func Wrap(kind Kind, cause error, message string) *Error {
	return &Error{Kind: kind, Message: message, Cause: cause}
}

// WithHint 返回一个附加了修复提示的副本.
func (e *Error) WithHint(hint string) *Error {
	clone := *e
	clone.Hint = hint
	return &clone
}

// WithHintf 返回一个附加了格式化修复提示的副本.
func (e *Error) WithHintf(format string, args ...any) *Error {
	return e.WithHint(fmt.Sprintf(format, args...))
}

// KindOf 提取任意错误的分类, 未分类的错误一律归为 KindGeneral.
func KindOf(err error) Kind {
	var target *Error
	if errors.As(err, &target) {
		return target.Kind
	}
	return KindGeneral
}

// ExitCodeOf 提取任意错误对应的退出码.
func ExitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	return KindOf(err).ExitCode()
}

// Detail 提取错误信封需要的三段信息.
func Detail(err error) (kind Kind, message string, hint string) {
	if err == nil {
		return KindGeneral, "", ""
	}
	var target *Error
	if errors.As(err, &target) {
		return target.Kind, target.Error(), target.Hint
	}
	return KindGeneral, err.Error(), ""
}

// 以下是本项目高频错误的构造入口, 集中在此处便于统一措辞.

// Usage 构造用法错误.
func Usage(message string) *Error { return New(KindUsage, message) }

// Usagef 构造格式化用法错误.
func Usagef(format string, args ...any) *Error { return Newf(KindUsage, format, args...) }

// Auth 构造认证错误.
func Auth(message string) *Error { return New(KindAuth, message) }

// NotFound 构造资源不存在错误.
func NotFound(message string) *Error { return New(KindNotFound, message) }

// Upstream 构造上游错误.
func Upstream(message string) *Error { return New(KindUpstream, message) }

// ReadOnly 构造只读模式拒绝错误.
func ReadOnly(message string) *Error { return New(KindReadOnly, message) }

// NonDestructive 构造非破坏性模式拒绝错误.
func NonDestructive(message string) *Error { return New(KindNonDestructive, message) }
