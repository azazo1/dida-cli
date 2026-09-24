package timeparse

import (
	"testing"
	"time"
)

func newResolver(t *testing.T, timezone string) *Resolver {
	t.Helper()
	resolver, err := New(timezone)
	if err != nil {
		t.Fatalf("构造解析器失败: %v", err)
	}
	return resolver
}

// TestParseProducesWireFormat 覆盖服务端要求的线格式.
//
// 线格式必须是 UTC 且偏移量不带冒号, 这两点任何一个错了服务端都会拒收,
// 而报错信息又很难定位, 所以在这里锁死.
func TestParseProducesWireFormat(t *testing.T) {
	resolver := newResolver(t, "Asia/Shanghai")
	cases := []struct {
		input string
		want  string
	}{
		{"2026-09-25", "2026-09-24T16:00:00.000+0000"},
		{"2026-09-25 09:00", "2026-09-25T01:00:00.000+0000"},
		{"2026-09-25 09:00:00", "2026-09-25T01:00:00.000+0000"},
		{"2026-09-25T09:00:00+08:00", "2026-09-25T01:00:00.000+0000"},
		{"2026-09-25T01:00:00.000+0000", "2026-09-25T01:00:00.000+0000"},
		{"2026-09-25T09:00:00Z", "2026-09-25T09:00:00.000+0000"},
	}
	for _, item := range cases {
		t.Run(item.input, func(t *testing.T) {
			wire, err := resolver.ParseWire(item.input)
			if err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			if wire != item.want {
				t.Fatalf("线格式 = %q, 期望 %q", wire, item.want)
			}
		})
	}
}

func TestParseRelativeDays(t *testing.T) {
	resolver := newResolver(t, "Asia/Shanghai")
	now := resolver.Now()
	todayMidnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, resolver.Location)

	cases := []struct {
		input string
		want  time.Time
	}{
		{"today", todayMidnight},
		{"tomorrow", todayMidnight.AddDate(0, 0, 1)},
		{"yesterday", todayMidnight.AddDate(0, 0, -1)},
		{"+3d", todayMidnight.AddDate(0, 0, 3)},
		{"-1w", todayMidnight.AddDate(0, 0, -7)},
		{"tomorrow 09:30", todayMidnight.AddDate(0, 0, 1).Add(9*time.Hour + 30*time.Minute)},
	}
	for _, item := range cases {
		t.Run(item.input, func(t *testing.T) {
			parsed, err := resolver.Parse(item.input)
			if err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			if !parsed.Equal(item.want) {
				t.Fatalf("解析结果 = %s, 期望 %s", parsed, item.want)
			}
		})
	}
}

func TestParseHourOffsetsAreRelativeToNow(t *testing.T) {
	resolver := newResolver(t, "Asia/Shanghai")
	before := resolver.Now()
	parsed, err := resolver.Parse("+2h")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	after := resolver.Now()
	// 小时偏移是相对当前时刻, 因此只能验证落在一个合理区间里.
	if parsed.Before(before.Add(2*time.Hour-time.Minute)) || parsed.After(after.Add(2*time.Hour+time.Minute)) {
		t.Fatalf("+2h 结果 %s 不在预期区间内", parsed)
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	resolver := newResolver(t, "Asia/Shanghai")
	for _, input := range []string{"", "   ", "明天", "2026-13-45", "+3x", "next week", "2026-09-25 09:00 09:00"} {
		if _, err := resolver.Parse(input); err == nil {
			t.Fatalf("输入 %q 本应被拒绝", input)
		}
	}
}

func TestEmptyInputStaysEmpty(t *testing.T) {
	resolver := newResolver(t, "Asia/Shanghai")
	wire, err := resolver.ParseWire("   ")
	if err != nil {
		t.Fatalf("空输入不应报错: %v", err)
	}
	if wire != "" {
		t.Fatalf("空输入应当返回空串, 实际 %q", wire)
	}
}

func TestUnknownTimezoneIsRejected(t *testing.T) {
	if _, err := New("Mars/Olympus"); err == nil {
		t.Fatal("未知时区本应被拒绝")
	}
}

func TestDayRangeCoversWholeDay(t *testing.T) {
	resolver := newResolver(t, "Asia/Shanghai")
	day, err := resolver.Parse("2026-09-25")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	start, end := resolver.DayRange(day)
	if start.Format("15:04:05") != "00:00:00" {
		t.Fatalf("起始时刻 = %s, 期望零点", start)
	}
	if end.Format("15:04:05") != "23:59:59" {
		t.Fatalf("结束时刻 = %s, 期望当日最后一秒", end)
	}
	if end.Sub(start) != 24*time.Hour-time.Second {
		t.Fatalf("区间长度 = %s", end.Sub(start))
	}
}
