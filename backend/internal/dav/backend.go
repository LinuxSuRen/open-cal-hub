// Package dav 实现 CalDAV 服务：以 emersion/go-webdav 的 caldav.Handler 为协议层，
// 后端映射到 calendars/events 表；Basic 认证（登录密码或应用密码）。
package dav

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"

	ics "github.com/emersion/go-ical"
	"github.com/emersion/go-webdav/caldav"

	"github.com/linuxsuren/open-cal-hub/backend/internal/cal"
)

const Prefix = "/dav"

// Principal 是认证后的当前用户。
type Principal struct {
	UserID   int64
	Username string
}

type principalKey struct{}

// WithPrincipal 注入当前用户到请求上下文。
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom 读取上下文当前用户。
func PrincipalFrom(ctx context.Context) (*Principal, error) {
	p, ok := ctx.Value(principalKey{}).(*Principal)
	if !ok || p == nil {
		return nil, errors.New("dav: no principal in context")
	}
	return p, nil
}

// Backend 实现 caldav.Backend。
type Backend struct {
	DB *sql.DB
}

// davError 携带 HTTP 状态；协议层无法识别时退化为 500（受上游库限制）。
type davError struct {
	code int
	msg  string
}

func (e *davError) Error() string { return fmt.Sprintf("dav: %d %s", e.code, e.msg) }

func errf(code int, format string, a ...any) error {
	return &davError{code: code, msg: fmt.Sprintf(format, a...)}
}

var errNotFound = errf(http.StatusNotFound, "not found")

func principalPath(username string) string { return Prefix + "/users/" + username + "/" }

func homeSetPath(username string) string { return principalPath(username) + "calendars/" }

// splitPath 去掉 /dav 前缀并拆分段。
func splitPath(p string) []string {
	p = path.Clean(p)
	p = strings.TrimPrefix(p, Prefix)
	return strings.Split(strings.Trim(p, "/"), "/")
}

// objectRef 是 .../{username}/calendars/{calendar_id}/{uid}.ics 的解析结果。
type objectRef struct {
	username   string
	calendarID int64
	uid        string
}

func parseObjectPath(p string) (*objectRef, bool) {
	segs := splitPath(p)
	if len(segs) != 5 || segs[0] != "users" || segs[2] != "calendars" {
		return nil, false
	}
	calID, err := strconv.ParseInt(segs[3], 10, 64)
	if err != nil || calID <= 0 {
		return nil, false
	}
	uid := segs[4]
	if !strings.HasSuffix(uid, ".ics") {
		return nil, false
	}
	return &objectRef{username: segs[1], calendarID: calID, uid: strings.TrimSuffix(uid, ".ics")}, true
}

func parseCalendarPath(p string) (username string, calID int64, ok bool) {
	segs := splitPath(p)
	if len(segs) != 4 || segs[0] != "users" || segs[2] != "calendars" {
		return "", 0, false
	}
	calID, err := strconv.ParseInt(segs[3], 10, 64)
	if err != nil || calID <= 0 {
		return "", 0, false
	}
	return segs[1], calID, true
}

// ownCalendar 校验日历属于当前用户。
func (b *Backend) ownCalendar(ctx context.Context, ref *objectRef) (*cal.Calendar, error) {
	p, err := PrincipalFrom(ctx)
	if err != nil {
		return nil, err
	}
	c, err := cal.GetCalendar(ctx, b.DB, ref.calendarID)
	if err != nil {
		return nil, errNotFound
	}
	if c.OwnerID != p.UserID {
		return nil, errNotFound
	}
	return c, nil
}

func (b *Backend) CalendarHomeSetPath(ctx context.Context) (string, error) {
	p, err := PrincipalFrom(ctx)
	if err != nil {
		return "", err
	}
	return homeSetPath(p.Username), nil
}

func (b *Backend) CurrentUserPrincipal(ctx context.Context) (string, error) {
	p, err := PrincipalFrom(ctx)
	if err != nil {
		return "", err
	}
	return principalPath(p.Username), nil
}

func (b *Backend) CreateCalendar(ctx context.Context, c *caldav.Calendar) error {
	p, err := PrincipalFrom(ctx)
	if err != nil {
		return err
	}
	segs := splitPath(c.Path)
	if len(segs) != 4 || segs[0] != "users" || segs[2] != "calendars" || segs[1] != p.Username {
		return errf(http.StatusForbidden, "calendar creation only allowed in own home set")
	}
	name := c.Name
	if name == "" {
		name = segs[3]
	}
	_, err = cal.CreateCalendar(ctx, b.DB, p.UserID, name, c.Description, "", "UTC")
	return err
}

func (b *Backend) ListCalendars(ctx context.Context) ([]caldav.Calendar, error) {
	p, err := PrincipalFrom(ctx)
	if err != nil {
		return nil, err
	}
	cals, err := cal.ListCalendarsByOwner(ctx, b.DB, p.UserID)
	if err != nil {
		return nil, err
	}
	out := make([]caldav.Calendar, 0, len(cals))
	for _, c := range cals {
		out = append(out, caldav.Calendar{
			Path:                  homeSetPath(p.Username) + strconv.FormatInt(c.ID, 10) + "/",
			Name:                  c.Name,
			Description:           c.Description,
			MaxResourceSize:       1 << 20,
			SupportedComponentSet: []string{ics.CompEvent},
		})
	}
	return out, nil
}

func (b *Backend) GetCalendar(ctx context.Context, p string) (*caldav.Calendar, error) {
	username, calID, ok := parseCalendarPath(p)
	if !ok {
		return nil, errNotFound
	}
	pr, err := PrincipalFrom(ctx)
	if err != nil {
		return nil, err
	}
	if username != pr.Username {
		return nil, errNotFound
	}
	c, err := cal.GetCalendar(ctx, b.DB, calID)
	if err != nil || c.OwnerID != pr.UserID {
		return nil, errNotFound
	}
	return &caldav.Calendar{
		Path:                  homeSetPath(pr.Username) + strconv.FormatInt(c.ID, 10) + "/",
		Name:                  c.Name,
		Description:           c.Description,
		MaxResourceSize:       1 << 20,
		SupportedComponentSet: []string{ics.CompEvent},
	}, nil
}

// mergedObject 把同一 UID 的行合成一个 CalDAV 资源（主事件 + 覆盖 VEVENT）。
func (b *Backend) mergedObject(ctx context.Context, username string, calendarID int64, rows []*cal.EventRow) (*caldav.CalendarObject, error) {
	if len(rows) == 0 {
		return nil, errNotFound
	}
	sort.SliceStable(rows, func(i, j int) bool {
		ri, rj := rows[i].RecurrenceID != nil, rows[j].RecurrenceID != nil
		if ri != rj {
			return !ri // 主事件在前
		}
		if ri && rj {
			return rows[i].RecurrenceID.Before(*rows[j].RecurrenceID)
		}
		return rows[i].ID < rows[j].ID
	})
	out := &caldav.CalendarObject{
		Path: fmt.Sprintf("%s%d/%s.ics", homeSetPath(username), calendarID, rows[0].UID),
	}
	calOut := ics.NewCalendar()
	calOut.Props.SetText(ics.PropProductID, cal.ProductID)
	calOut.Props.SetText(ics.PropVersion, "2.0")
	for _, r := range rows {
		if out.ModTime.IsZero() || r.UpdatedAt.After(out.ModTime) {
			out.ModTime = r.UpdatedAt
		}
		if r.RecurrenceID == nil {
			out.ETag = r.ETag
		}
		src, err := ics.NewDecoder(strings.NewReader(r.Data)).Decode()
		if err != nil {
			continue
		}
		for _, child := range src.Children {
			if child.Name == ics.CompEvent {
				calOut.Children = append(calOut.Children, child)
			}
		}
	}
	if out.ETag == "" {
		out.ETag = rows[0].ETag
	}
	out.Data = calOut
	return out, nil
}

func (b *Backend) GetCalendarObject(ctx context.Context, p string, req *caldav.CalendarCompRequest) (*caldav.CalendarObject, error) {
	ref, ok := parseObjectPath(p)
	if !ok {
		return nil, errNotFound
	}
	c, err := b.ownCalendar(ctx, ref)
	if err != nil {
		return nil, err
	}
	rows, err := cal.EventsByUID(ctx, b.DB, c.ID, ref.uid)
	if err != nil {
		return nil, err
	}
	pr, _ := PrincipalFrom(ctx)
	return b.mergedObject(ctx, pr.Username, c.ID, rows)
}

func (b *Backend) ListCalendarObjects(ctx context.Context, p string, req *caldav.CalendarCompRequest) ([]caldav.CalendarObject, error) {
	username, calID, ok := parseCalendarPath(p)
	if !ok {
		return nil, errNotFound
	}
	pr, err := PrincipalFrom(ctx)
	if err != nil {
		return nil, err
	}
	if username != pr.Username {
		return nil, errNotFound
	}
	c, err := cal.GetCalendar(ctx, b.DB, calID)
	if err != nil || c.OwnerID != pr.UserID {
		return nil, errNotFound
	}
	all, err := loadCalendarRows(ctx, b.DB, c.ID)
	if err != nil {
		return nil, err
	}
	var out []caldav.CalendarObject
	for _, rows := range groupByUID(all) {
		obj, err := b.mergedObject(ctx, pr.Username, c.ID, rows)
		if err != nil {
			continue
		}
		out = append(out, *obj)
	}
	return out, nil
}

func (b *Backend) QueryCalendarObjects(ctx context.Context, p string, query *caldav.CalendarQuery) ([]caldav.CalendarObject, error) {
	cos, err := b.ListCalendarObjects(ctx, p, nil)
	if err != nil {
		return nil, err
	}
	if query == nil {
		return cos, nil
	}
	return caldav.Filter(query, cos)
}

func (b *Backend) PutCalendarObject(ctx context.Context, p string, vcal *ics.Calendar, opts *caldav.PutCalendarObjectOptions) (*caldav.CalendarObject, error) {
	ref, ok := parseObjectPath(p)
	if !ok {
		return nil, errNotFound
	}
	c, err := b.ownCalendar(ctx, ref)
	if err != nil {
		return nil, err
	}
	eventType, uid, err := caldav.ValidateCalendarObject(vcal)
	if err != nil {
		return nil, errf(http.StatusBadRequest, "invalid calendar object: %v", err)
	}
	if eventType != ics.CompEvent {
		return nil, errf(http.StatusBadRequest, "only VEVENT supported")
	}
	if uid == "" {
		return nil, errf(http.StatusBadRequest, "missing UID")
	}
	if uid != ref.uid {
		return nil, errf(http.StatusBadRequest, "UID conflicts with resource path")
	}
	existing, err := cal.EventsByUID(ctx, b.DB, c.ID, uid)
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 {
		if opts != nil && opts.IfNoneMatch.IsWildcard() {
			return nil, errf(http.StatusPreconditionFailed, "resource exists")
		}
		if opts != nil && opts.IfMatch != "" {
			curETag := ""
			for _, r := range existing {
				if r.RecurrenceID == nil {
					curETag = r.ETag
				}
			}
			if curETag == "" {
				curETag = existing[0].ETag
			}
			match, merr := opts.IfMatch.MatchETag(curETag)
			if merr != nil || !match {
				return nil, errf(http.StatusPreconditionFailed, "etag mismatch")
			}
		}
	}
	// 拆分主事件与覆盖并整体重建
	var masters, overrides []*ics.Component
	for _, comp := range vcal.Children {
		if comp.Name != ics.CompEvent {
			continue
		}
		if comp.Props.Get(ics.PropRecurrenceID) != nil {
			overrides = append(overrides, comp)
		} else {
			masters = append(masters, comp)
		}
	}
	if len(masters) != 1 {
		return nil, errf(http.StatusBadRequest, "exactly one VEVENT without RECURRENCE-ID required")
	}
	if err := cal.DeleteEventsByUID(ctx, b.DB, c.ID, uid); err != nil {
		return nil, err
	}
	seq := 0
	if len(existing) > 0 {
		for _, r := range existing {
			if r.Sequence > seq {
				seq = r.Sequence
			}
		}
		seq++
	}
	insert := func(comp *ics.Component) error {
		single := ics.NewCalendar()
		single.Props.SetText(ics.PropProductID, cal.ProductID)
		single.Props.SetText(ics.PropVersion, "2.0")
		single.Children = append(single.Children, comp)
		data, err := cal.EncodeCalendar(single)
		if err != nil {
			return err
		}
		d, err := cal.DeriveEvent(comp, "UTC")
		if err != nil {
			return errf(http.StatusBadRequest, "invalid VEVENT: %v", err)
		}
		row := &cal.EventRow{
			CalendarID: c.ID, UID: uid, Data: data,
			Summary: d.Summary, Location: d.Location,
			Start: d.Start, End: d.End, TZ: d.TZ, AllDay: d.AllDay,
			RRule: d.RRule, Reminders: d.Reminders, Status: d.Status,
			Sequence: seq, RecurrenceID: d.RecurrenceID,
		}
		return cal.InsertEventRow(ctx, b.DB, row)
	}
	if err := insert(masters[0]); err != nil {
		return nil, err
	}
	for _, comp := range overrides {
		if err := insert(comp); err != nil {
			return nil, err
		}
	}
	rows, err := cal.EventsByUID(ctx, b.DB, c.ID, uid)
	if err != nil {
		return nil, err
	}
	pr, _ := PrincipalFrom(ctx)
	return b.mergedObject(ctx, pr.Username, c.ID, rows)
}

func (b *Backend) DeleteCalendarObject(ctx context.Context, p string) error {
	ref, ok := parseObjectPath(p)
	if !ok {
		return errNotFound
	}
	c, err := b.ownCalendar(ctx, ref)
	if err != nil {
		return err
	}
	return cal.DeleteEventsByUID(ctx, b.DB, c.ID, ref.uid)
}

func loadCalendarRows(ctx context.Context, q cal.DBTX, calendarID int64) ([]*cal.EventRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+cal.EventColumns()+` FROM events WHERE calendar_id = ? ORDER BY id`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return cal.ScanEventRows(rows)
}

func groupByUID(rows []*cal.EventRow) map[string][]*cal.EventRow {
	out := map[string][]*cal.EventRow{}
	for _, r := range rows {
		out[r.UID] = append(out[r.UID], r)
	}
	return out
}
