package cal

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	ics "github.com/emersion/go-ical"
	"github.com/teambition/rrule-go"
)

// ProductID 是本服务生成的 iCalendar 对象的 PRODID。
const ProductID = "-//CalHub//Open Cal Hub 1.0//CN"

// MaxReminders 单事件提醒数量上限；MaxReminderMinutes 提醒提前量上限（30 天）。
const (
	MaxReminders       = 10
	MaxReminderMinutes = 30 * 24 * 60
)

// EventInput 是创建/更新事件的规范输入（POST/PATCH 归一后、CalDAV 之外的写入路径共用）。
type EventInput struct {
	Summary     string
	Location    string
	Description string
	Start       time.Time // UTC
	End        time.Time // UTC
	AllDay      bool
	TZ          string // IANA 名称；all_day 时忽略
	Status      string // confirmed|tentative|cancelled
	Reminders   []int  // 提前分钟数
	RRule       string // 规范化 RRULE 文本；空串表示不重复
}

// BuildVCalendar 由规范输入构建完整 VCALENDAR 文本（不含 METHOD，符合 CalDAV 存储要求）。
func BuildVCalendar(uid string, in EventInput, sequence int, stamp time.Time) (*ics.Calendar, error) {
	loc := time.UTC
	if !in.AllDay && in.TZ != "" && in.TZ != "UTC" {
		l, err := time.LoadLocation(in.TZ)
		if err != nil {
			return nil, fmt.Errorf("invalid timezone %q: %w", in.TZ, err)
		}
		loc = l
	}
	ev := ics.NewEvent()
	ev.Props.SetText(ics.PropUID, uid)
	ev.Props.SetDateTime(ics.PropDateTimeStamp, stamp.UTC())
	if sequence > 0 {
		ev.Props.SetText(ics.PropSequence, strconv.Itoa(sequence))
	}
	if in.Summary != "" {
		ev.Props.SetText(ics.PropSummary, in.Summary)
	}
	if in.Location != "" {
		ev.Props.SetText(ics.PropLocation, in.Location)
	}
	if in.Description != "" {
		ev.Props.SetText(ics.PropDescription, in.Description)
	}
	if in.AllDay {
		ev.Props.SetDate(ics.PropDateTimeStart, in.Start.UTC())
		ev.Props.SetDate(ics.PropDateTimeEnd, in.End.UTC())
	} else {
		ev.Props.SetDateTime(ics.PropDateTimeStart, in.Start.In(loc))
		ev.Props.SetDateTime(ics.PropDateTimeEnd, in.End.In(loc))
	}
	if in.Status != "" && in.Status != StatusConfirmed {
		ev.Props.SetText(ics.PropStatus, strings.ToUpper(in.Status))
	}
	if in.RRule != "" {
		// RRULE 的值类型是 RECUR：不能走 SetText（会加 VALUE=TEXT 并转义分号，
		// 导致 RecurrenceSet 解析失败、重复事件无法展开——见 cal/expand_test.go 回归）。
		ev.Props.Set(&ics.Prop{Name: ics.PropRecurrenceRule, Value: in.RRule})
	}
	for _, m := range sortedUnique(in.Reminders) {
		alarm := ics.NewComponent(ics.CompAlarm)
		alarm.Props.SetText(ics.PropAction, "DISPLAY")
		alarm.Props.SetText(ics.PropTrigger, formatTrigger(m))
		alarm.Props.SetText(ics.PropDescription, in.Summary)
		ev.Children = append(ev.Children, alarm)
	}
	cal := ics.NewCalendar()
	cal.Props.SetText(ics.PropProductID, ProductID)
	cal.Props.SetText(ics.PropVersion, "2.0")
	cal.Props.SetText(ics.PropCalendarScale, "GREGORIAN")
	cal.Children = append(cal.Children, ev.Component)
	return cal, nil
}

// EncodeCalendar 序列化为 CRLF 文本。
func EncodeCalendar(c *ics.Calendar) (string, error) {
	var sb strings.Builder
	if err := ics.NewEncoder(&crlfWriter{w: &sb}).Encode(c); err != nil {
		return "", err
	}
	return sb.String(), nil
}

func sortedUnique(in []int) []int {
	seen := make(map[int]bool, len(in))
	out := make([]int, 0, len(in))
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Ints(out)
	return out
}

func formatTrigger(minutes int) string {
	if minutes == 0 {
		return "PT0S"
	}
	neg := "-"
	v := minutes
	if v < 0 {
		neg = ""
		v = -v
	}
	switch {
	case v%1440 == 0:
		return fmt.Sprintf("%sP%dD", neg, v/1440)
	case v%60 == 0:
		return fmt.Sprintf("%sPT%dH", neg, v/60)
	default:
		return fmt.Sprintf("%sPT%dM", neg, v)
	}
}

func parseTrigger(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	neg := s[0] == '-'
	if neg || s[0] == '+' {
		s = s[1:]
	}
	if !strings.HasPrefix(s, "P") {
		return 0, false
	}
	var total time.Duration
	isTime := false
	num := 0
	haveNum := false
	flush := func(unit string) bool {
		if !haveNum {
			return false
		}
		switch unit {
		case "D", "W":
			days := num
			if unit == "W" {
				days *= 7
			}
			total += time.Duration(days) * 24 * time.Hour
		case "H":
			total += time.Duration(num) * time.Hour
		case "M":
			total += time.Duration(num) * time.Minute
		case "S":
			total += time.Duration(num) * time.Second
		default:
			return false
		}
		num, haveNum = 0, false
		return true
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		switch {
		case c == 'T':
			isTime = true
		case c >= '0' && c <= '9':
			num = num*10 + int(c-'0')
			haveNum = true
		case c == 'D':
			if isTime || !flush("D") {
				return 0, false
			}
		case c == 'W':
			if isTime || !flush("W") {
				return 0, false
			}
		case c == 'H' || c == 'M' || c == 'S':
			if !isTime || !flush(string(c)) {
				return 0, false
			}
		default:
			return 0, false
		}
	}
	if haveNum {
		return 0, false
	}
	minutes := int(total / time.Minute)
	if total%time.Minute != 0 {
		return 0, false
	}
	if neg {
		minutes = -minutes
	}
	return minutes, true
}

// Derived 是从 VEVENT 原文解析出的索引字段。
type Derived struct {
	UID          string
	RecurrenceID *time.Time // 实例覆盖时非空
	Summary      string
	Location     string
	Description  string
	Start        time.Time // UTC
	End          time.Time // UTC
	TZ           string
	AllDay       bool
	RRule        string
	Status       string
	Reminders    []int
}

// DeriveEvent 从单个 VEVENT 解析索引字段。loc 是缺省时区（无 TZID 时使用）。
func DeriveEvent(ev *ics.Component, fallbackTZ string) (*Derived, error) {
	if ev == nil || ev.Name != ics.CompEvent {
		return nil, fmt.Errorf("not a VEVENT")
	}
	d := &Derived{Status: StatusConfirmed, TZ: "UTC"}
	if v, err := ev.Props.Text(ics.PropUID); err == nil {
		d.UID = v
	}
	if p := ev.Props.Get(ics.PropRecurrenceID); p != nil {
		t, err := propTime(p, fallbackTZ)
		if err != nil {
			return nil, fmt.Errorf("parse RECURRENCE-ID: %w", err)
		}
		u := t.UTC()
		d.RecurrenceID = &u
	}
	if v, err := ev.Props.Text(ics.PropSummary); err == nil {
		d.Summary = v
	}
	if v, err := ev.Props.Text(ics.PropLocation); err == nil {
		d.Location = v
	}
	if v, err := ev.Props.Text(ics.PropDescription); err == nil {
		d.Description = v
	}
	if p := ev.Props.Get(ics.PropDateTimeStart); p != nil {
		t, err := propTime(p, fallbackTZ)
		if err != nil {
			return nil, fmt.Errorf("parse DTSTART: %w", err)
		}
		d.Start = t.UTC()
		d.AllDay = p.ValueType() == ics.ValueDate || len(p.Value) == 8
		if tzid := p.Params.Get(ics.PropTimezoneID); tzid != "" {
			d.TZ = tzid
		}
	} else {
		return nil, fmt.Errorf("VEVENT missing DTSTART")
	}
	if p := ev.Props.Get(ics.PropDateTimeEnd); p != nil {
		t, err := propTime(p, fallbackTZ)
		if err != nil {
			return nil, fmt.Errorf("parse DTEND: %w", err)
		}
		d.End = t.UTC()
	} else if p := ev.Props.Get(ics.PropDuration); p != nil {
		dur, err := p.Duration()
		if err != nil {
			return nil, fmt.Errorf("parse DURATION: %w", err)
		}
		d.End = d.Start.Add(dur).UTC()
	} else if d.AllDay {
		d.End = d.Start.Add(24 * time.Hour).UTC()
	} else {
		d.End = d.Start.UTC()
	}
	if p := ev.Props.Get(ics.PropRecurrenceRule); p != nil && p.Value != "" {
		d.RRule = p.Value
		// 兼容历史 VALUE=TEXT 写法（分号被转义）
		d.RRule = strings.ReplaceAll(d.RRule, `\;`, ";")
	}
	if v, err := ev.Props.Text(ics.PropStatus); err == nil {
		switch strings.ToLower(v) {
		case StatusTentative:
			d.Status = StatusTentative
		case StatusCancelled:
			d.Status = StatusCancelled
		}
	}
	for _, child := range ev.Children {
		if child.Name != ics.CompAlarm {
			continue
		}
		if a, _ := child.Props.Text(ics.PropAction); !strings.EqualFold(a, "DISPLAY") {
			continue
		}
		tp := child.Props.Get(ics.PropTrigger)
		if tp == nil {
			continue
		}
		if m, ok := parseTrigger(tp.Value); ok && m >= 0 && m <= MaxReminderMinutes {
			d.Reminders = append(d.Reminders, m)
		}
	}
	d.Reminders = sortedUnique(d.Reminders)
	if len(d.Reminders) > MaxReminders {
		d.Reminders = d.Reminders[:MaxReminders]
	}
	if d.AllDay {
		d.TZ = "UTC"
	}
	return d, nil
}

// propTime 解析属性值为时间：支持 DATE、本地时间+TZID、UTC 时间。
func propTime(p *ics.Prop, fallbackTZ string) (time.Time, error) {
	value := p.Value
	vt := p.ValueType()
	switch {
	case vt == ics.ValueDate || len(value) == 8:
		return time.ParseInLocation("20060102", value, time.UTC)
	case len(value) == 16:
		loc, err := tzLocation(p.Params.Get(ics.PropTimezoneID), fallbackTZ)
		if err != nil {
			return time.Time{}, err
		}
		return time.ParseInLocation("20060102T150405", value, loc)
	default:
		return time.Parse(time.RFC3339, value)
	}
}

func tzLocation(tzid, fallback string) (*time.Location, error) {
	name := tzid
	if name == "" {
		name = fallback
	}
	if name == "" || name == "UTC" {
		return time.UTC, nil
	}
	l, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("unknown timezone %q: %w", name, err)
	}
	return l, nil
}

// RRuleInput 是契约 §3 的 rrule 对象。
type RRuleInput struct {
	Freq     string   `json:"freq"`
	Interval int      `json:"interval"`
	Byday    []string `json:"byday"`
	Until    string   `json:"until"`
	Count    int      `json:"count"`
}

// ParseRRuleInput 解析请求中的 rrule 字段（JSON 原文）；
// 返回规范文本；clear 表示显式 null（用于 PATCH 清除重复规则）。
func ParseRRuleInput(raw json.RawMessage) (rruleText string, clear bool, err error) {
	if len(raw) == 0 {
		return "", false, nil
	}
	if string(raw) == "null" {
		return "", true, nil
	}
	var in RRuleInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", false, err
	}
	text, err := CanonicalRRule(&in)
	if err != nil {
		return "", false, err
	}
	return text, false, nil
}

// CanonicalRRule 将契约 rrule 对象转为规范 RRULE 文本并校验。
func CanonicalRRule(in *RRuleInput) (string, error) {
	if in == nil {
		return "", nil
	}
	opt := rrule.ROption{}
	switch strings.ToUpper(in.Freq) {
	case "DAILY":
		opt.Freq = rrule.DAILY
	case "WEEKLY":
		opt.Freq = rrule.WEEKLY
	case "MONTHLY":
		opt.Freq = rrule.MONTHLY
	case "YEARLY":
		opt.Freq = rrule.YEARLY
	default:
		return "", fmt.Errorf("freq must be one of DAILY|WEEKLY|MONTHLY|YEARLY")
	}
	if in.Interval < 0 {
		return "", fmt.Errorf("interval must be >= 0")
	}
	if in.Interval > 1 {
		opt.Interval = in.Interval
	}
	for _, d := range in.Byday {
		wd, nth, err := parseByday(d)
		if err != nil {
			return "", err
		}
		if nth != 0 {
			opt.Byweekday = append(opt.Byweekday, wd.Nth(nth))
		} else {
			opt.Byweekday = append(opt.Byweekday, wd)
		}
	}
	if in.Count > 0 && in.Until != "" {
		return "", fmt.Errorf("count and until are mutually exclusive")
	}
	if in.Count > 0 {
		opt.Count = in.Count
		if opt.Count > MaxExpandInstances {
			return "", fmt.Errorf("count too large (max %d)", MaxExpandInstances)
		}
	}
	if in.Until != "" {
		// 兼容 RFC3339（带连字符，前端提交格式）与 iCalendar 紧凑格式（20060102T150405Z）
		u, err := time.Parse(time.RFC3339, in.Until)
		if err != nil {
			if u, err = time.Parse("20060102T150405Z", in.Until); err != nil {
				return "", fmt.Errorf("until must be RFC3339")
			}
		}
		opt.Until = u.UTC()
	}
	r, err := rrule.NewRRule(opt)
	if err != nil {
		return "", err
	}
	return r.String(), nil
}

var weekdayByName = map[string]rrule.Weekday{
	"MO": rrule.MO, "TU": rrule.TU, "WE": rrule.WE, "TH": rrule.TH,
	"FR": rrule.FR, "SA": rrule.SA, "SU": rrule.SU,
}

func parseByday(s string) (rrule.Weekday, int, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	n := ""
	i := 0
	for i < len(s) && (s[i] == '-' || s[i] == '+' || (s[i] >= '0' && s[i] <= '9')) {
		n += string(s[i])
		i++
	}
	wd, ok := weekdayByName[s[i:]]
	if !ok {
		return rrule.MO, 0, fmt.Errorf("invalid BYDAY %q", s)
	}
	nth := 0
	if n != "" && n != "+" {
		v, err := strconv.Atoi(n)
		if err != nil || v == 0 || v < -5 || v > 5 {
			return rrule.MO, 0, fmt.Errorf("invalid BYDAY ordinal %q", s)
		}
		nth = v
	}
	return wd, nth, nil
}

// crlfWriter 让 ical 编码器输出 CRLF（编码器写 \n，这里统一替换）。
type crlfWriter struct{ w *strings.Builder }

func (c *crlfWriter) Write(p []byte) (int, error) {
	s := strings.ReplaceAll(string(p), "\n", "\r\n")
	s = strings.ReplaceAll(s, "\r\r", "\r")
	return c.w.WriteString(s)
}
