package config

import (
	"os"
	"strings"
	"time"

	"github.com/azazo1/dida-cli/internal/apperr"
)

// TokenSource 描述口令的来源.
type TokenSource string

const (
	// TokenSourceEnv 表示口令来自环境变量.
	TokenSourceEnv TokenSource = "env"
	// TokenSourceFile 表示口令来自本地口令文件.
	TokenSourceFile TokenSource = "file"
	// TokenSourceMissing 表示没有找到口令.
	TokenSourceMissing TokenSource = "missing"
)

// TokenStatus 是口令状态的脱敏描述, 可以安全地打印或写进 JSON.
type TokenStatus struct {
	Available bool        `json:"available"`
	Source    TokenSource `json:"source"`
	Preview   string      `json:"preview,omitempty"`
	Path      string      `json:"path,omitempty"`
	SavedAt   string      `json:"saved_at,omitempty"`
	Mode      string      `json:"mode,omitempty"`
	// Permissive 表示口令文件权限过宽, 存在被同机其他用户读取的风险.
	Permissive bool `json:"permissive,omitempty"`
}

// ResolveToken 按环境变量优先的顺序取出可用口令.
func ResolveToken() (string, TokenStatus, error) {
	if value := normalizeToken(os.Getenv(EnvToken)); value != "" {
		status := TokenStatus{
			Available: true,
			Source:    TokenSourceEnv,
			Preview:   RedactToken(value),
		}
		return value, status, nil
	}
	value, status, err := ReadTokenFile()
	if err != nil {
		return "", status, err
	}
	if value == "" {
		return "", status, apperr.Auth("no dida token found").
			WithHint("run: printf 'dp_...' | dida auth login --token-stdin")
	}
	return value, status, nil
}

// ReadTokenFile 读取口令文件, 不存在时返回空值与 missing 状态.
func ReadTokenFile() (string, TokenStatus, error) {
	path := TokenPath()
	status := TokenStatus{Source: TokenSourceMissing, Path: path}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", status, nil
		}
		return "", status, apperr.Wrap(apperr.KindGeneral, err, "stat token file failed")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", status, apperr.Wrap(apperr.KindGeneral, err, "read token file failed").
			WithHintf("check permissions on %s", path)
	}
	token := parseTokenContent(string(raw))
	if token == "" {
		status.Source = TokenSourceFile
		status.SavedAt = info.ModTime().Format(time.RFC3339)
		status.Mode = formatMode(info.Mode())
		return "", status, apperr.Auth("token file exists but contains no usable token").
			WithHintf("rewrite %s with a single line starting with dp_", path)
	}
	status.Available = true
	status.Source = TokenSourceFile
	status.Preview = RedactToken(token)
	status.SavedAt = info.ModTime().Format(time.RFC3339)
	status.Mode = formatMode(info.Mode())
	status.Permissive = info.Mode().Perm()&0o077 != 0
	return token, status, nil
}

// SaveToken 把口令写入本地口令文件, 文件权限固定为 0600.
func SaveToken(token string) error {
	value := normalizeToken(token)
	if value == "" {
		return apperr.Usage("empty token, nothing to save")
	}
	if !strings.HasPrefix(value, "dp_") {
		return apperr.Usage("token does not look like a dida api token, expected a dp_ prefix").
			WithHint("copy the token from 滴答清单 web app: avatar > settings > account > api token")
	}
	if err := EnsureDir(); err != nil {
		return err
	}
	path := TokenPath()
	if err := os.WriteFile(path, []byte(value+"\n"), 0o600); err != nil {
		return apperr.Wrap(apperr.KindGeneral, err, "write token file failed")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return apperr.Wrap(apperr.KindGeneral, err, "tighten token file permission failed")
	}
	return nil
}

// ClearToken 删除本地口令文件, 文件不存在时视为成功.
func ClearToken() error {
	path := TokenPath()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return apperr.Wrap(apperr.KindGeneral, err, "remove token file failed")
	}
	return nil
}

// RedactToken 把口令裁剪为只保留首尾的预览形式.
func RedactToken(token string) string {
	value := normalizeToken(token)
	if value == "" {
		return ""
	}
	if len(value) <= 12 {
		return "***"
	}
	return value[:4] + "..." + value[len(value)-4:]
}

// parseTokenContent 从文件内容中取出第一行有效口令, 忽略空行与注释行.
func parseTokenContent(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		return trimmed
	}
	return ""
}

func normalizeToken(token string) string {
	return strings.TrimSpace(token)
}

func formatMode(mode os.FileMode) string {
	return "0" + itoa(int(mode.Perm())/64%8) + itoa(int(mode.Perm())/8%8) + itoa(int(mode.Perm())%8)
}
