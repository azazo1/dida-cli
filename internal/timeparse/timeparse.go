// Package timeparse 负责把用户与 agent 输入的时间文本转换成滴答服务端要求的线格式.
//
// 服务端接受的实测格式是 UTC 毫秒精度且偏移量不带冒号, 例如
// 2026-09-25T01:00:00.000+0000. 不带时区的输入按配置时区解释.
package timeparse

import (
	"strings"
	"time"

	"github.com/azazo1/dida-cli/internal/apperr"
)

// WireLayout 是滴答服务端的时间线格式.
const WireLayout = "2006-01-02T15:04:05.000+0000"

// DateLayout 是纯日期格式.
const DateLayout = "2006-01-02"

// 不带时区的输入格式, 按配置时区解释.
var zonelessLayouts = []string{
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02",
}

// 自带时区的输入格式.
var zonedLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.000+0000",
	"2006-01-02T15:04:05-0700",
	"2006-01-02T15:04:05Z0700",
}

// Resolver 持有时间解析所需的时区上下文.
type Resolver struct {
	// Location 是解释无时区输入时使用的时区.
	Location *time.Location
}

// New 构造一个解析器, timezone 为空时使用系统本地时区.
func New(timezone string) (*Resolver, error) {
	name := strings.TrimSpace(timezone)
	if name == "" {
		return &Resolver{Location: time.Local}, nil
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, apperr.Usagef("unknown timezone %q", name).
			WithHint("use an IANA timezone name such as Asia/Shanghai, or run: dida config set timezone Asia/Shanghai")
	}
	return &Resolver{Location: location}, nil
}

// Now 返回解析器时区下的当前时间.
func (r *Resolver) Now() time.Time { return time.Now().In(r.Location) }

// Wire 把时间转成服务端线格式.
func Wire(value time.Time) string { return value.UTC().Format(WireLayout) }

// Date 把时间转成本地日期文本.
func (r *Resolver) Date(value time.Time) string { return value.In(r.Location).Format(DateLayout) }

// DayRange 返回某一天的起止时间, 用于按日查询.
func (r *Resolver) DayRange(day time.Time) (start time.Time, end time.Time) {
	local := day.In(r.Location)
	start = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, r.Location)
	return start, start.AddDate(0, 0, 1).Add(-time.Second)
}

// Parse 解析一个时间输入.
//
// 支持的写法:
//   - RFC3339, 例如 2026-09-25T09:00:00+08:00
//   - 2026-09-25, 2026-09-25 09:00, 2026-09-25T09:00
//   - 服务端线格式, 例如 2026-09-25T01:00:00.000+0000
//   - 相对写法: today, tomorrow, yesterday, +3d, -1w, +2h
//   - 相对写法后接时间: "tomorrow 09:00", "+3d 18:30"
//
// 纯日期与纯相对天数的结果落在当地时间的 00:00.
func (r *Resolver) Parse(input string) (time.Time, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return time.Time{}, apperr.Usage("empty time value")
	}
	for _, layout := range zonelessLayouts {
		if parsed, err := time.ParseInLocation(layout, value, r.Location); err == nil && parsed.Format(layout) == value {
			return parsed, nil
		}
	}
	for _, layout := range zonedLayouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.In(r.Location), nil
		}
	}
	return r.parseRelative(value)
}

// ParseWire 解析并把结果转成服务端线格式, 空输入返回空串.
func (r *Resolver) ParseWire(input string) (string, error) {
	if strings.TrimSpace(input) == "" {
		return "", nil
	}
	parsed, err := r.Parse(input)
	if err != nil {
		return "", err
	}
	return Wire(parsed), nil
}

// parseRelative 解析相对时间写法.
func (r *Resolver) parseRelative(value string) (time.Time, error) {
	fields := strings.Fields(value)
	if len(fields) == 0 || len(fields) > 2 {
		return time.Time{}, r.invalidTimeError(value)
	}
	base, err := r.relativeBase(fields[0])
	if err != nil {
		return time.Time{}, err
	}
	if len(fields) == 1 {
		return base, nil
	}
	// 相对写法后接具体时刻, 例如 "tomorrow 09:00".
	for _, layout := range []string{"15:04:05", "15:04"} {
		if clock, err := time.ParseInLocation(layout, fields[1], r.Location); err == nil {
			return time.Date(base.Year(), base.Month(), base.Day(), clock.Hour(), clock.Minute(), clock.Second(), 0, r.Location), nil
		}
	}
	return time.Time{}, r.invalidTimeError(value)
}

// relativeBase 解析相对基准日.
func (r *Resolver) relativeBase(token string) (time.Time, error) {
	now := r.Now()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, r.Location)
	switch strings.ToLower(token) {
	case "today":
		return midnight, nil
	case "tomorrow":
		return midnight.AddDate(0, 0, 1), nil
	case "yesterday":
		return midnight.AddDate(0, 0, -1), nil
	}
	// 形如 +3d / -1w / +2h 的偏移写法.
	if len(token) >= 3 && (token[0] == '+' || token[0] == '-') {
		amount, ok := parseAmount(token[1 : len(token)-1])
		if !ok {
			return time.Time{}, r.invalidTimeError(token)
		}
		if token[0] == '-' {
			amount = -amount
		}
		switch token[len(token)-1] {
		case 'd':
			return midnight.AddDate(0, 0, amount), nil
		case 'w':
			return midnight.AddDate(0, 0, amount*7), nil
		case 'h':
			return now.Add(time.Duration(amount) * time.Hour), nil
		case 'm':
			return now.Add(time.Duration(amount) * time.Minute), nil
		}
	}
	return time.Time{}, r.invalidTimeError(token)
}

func parseAmount(text string) (int, bool) {
	if text == "" {
		return 0, false
	}
	value := 0
	for _, char := range text {
		if char < '0' || char > '9' {
			return 0, false
		}
		value = value*10 + int(char-'0')
	}
	return value, true
}

func (r *Resolver) invalidTimeError(value string) error {
	return apperr.Usagef("cannot parse time %q", value).
		WithHint("accepted forms: 2026-09-25, 2026-09-25 09:00, 2026-09-25T09:00:00+08:00, today, tomorrow, +3d, -1w, \"tomorrow 09:00\"")
}
