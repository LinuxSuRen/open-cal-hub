// 组 6：CalDAV（API 级）——PROPFIND(Depth:1) 列日历、PUT VEVENT、
// REPORT time-range 返回 207 含事件、GET 对象 ETag。
import { test, expect } from '@playwright/test';
import { ensureUser, PASSWORD, BASE } from './utils/api';

const user = 'dave'; // 全轮共享用户（限流 5/h/IP，见 utils/api.ts）；密码见 PASSWORD

function davHeaders(extra: Record<string, string> = {}): Record<string, string> {
  const auth = Buffer.from(`${user}:${PASSWORD}`).toString('base64');
  return { authorization: `Basic ${auth}`, ...extra };
}

async function davFetch(method: string, path: string, body?: string, headers: Record<string, string> = {}) {
  return fetch(BASE() + path, {
    method,
    headers: davHeaders(body ? { 'content-type': 'text/xml; charset=utf-8', ...headers } : headers),
    body,
  });
}

function vevent(uid: string, day: string): string {
  return [
    'BEGIN:VCALENDAR', 'VERSION:2.0', 'PRODID:-//E2E//CalDAV//CN',
    'BEGIN:VEVENT', `UID:${uid}`, `DTSTAMP:20261114T000000Z`,
    `DTSTART;TZID=Asia/Shanghai:${day}T090000`, `DTEND;TZID=Asia/Shanghai:${day}T100000`,
    `SUMMARY:CalDAV 事件`, 'END:VEVENT', 'END:VCALENDAR', '',
  ].join('\r\n');
}

test.describe.serial('CalDAV 协议', () => {
  let calPath: string;

  test('PROPFIND(Depth:1) 列出默认日历', async () => {
    await ensureUser(user); // 注册钩子创建默认日历
    const propfind = `<?xml version="1.0" encoding="utf-8"?>
<d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">
  <d:prop><d:resourcetype/><d:displayname/><c:supported-calendar-component-set/></d:prop>
</d:propfind>`;
    const res = await davFetch('PROPFIND', `/dav/users/${user}/calendars/`, propfind, { depth: '1' });
    expect(res.status).toBe(207);
    const body = await res.text();
    expect(body).toContain(`/dav/users/${user}/calendars/`);
    // 从响应提取日历集合路径（形如 /dav/users/davy/calendars/<id>/）
    const m = body.match(new RegExp(`/dav/users/${user}/calendars/(\\d+)/`));
    expect(m).toBeTruthy();
    calPath = `/dav/users/${user}/calendars/${m![1]}/`;
  });

  test('PUT VEVENT → 201；GET 对象返回 200 与 ETag', async () => {
    const uid = 'e2e-caldav-001';
    const put = await davFetch('PUT', `${calPath}${uid}.ics`, vevent(uid, '20261218'), {
      'content-type': 'text/calendar; charset=utf-8',
    });
    expect(put.status).toBe(201);
    expect(put.headers.get('etag')).toBeTruthy();

    const get = await davFetch('GET', `${calPath}${uid}.ics`);
    expect(get.status).toBe(200);
    expect(get.headers.get('content-type')).toContain('text/calendar');
    expect(await get.text()).toContain(uid);
  });

  test('REPORT time-range 返回 207 且含事件', async () => {
    const report = `<?xml version="1.0" encoding="utf-8"?>
<c:calendar-query xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">
  <d:prop><d:getetag/><c:calendar-data/></d:prop>
  <c:filter><c:comp-filter name="VCALENDAR">
    <c:comp-filter name="VEVENT">
      <c:time-range start="20261201T000000Z" end="20261231T000000Z"/>
    </c:comp-filter>
  </c:comp-filter></c:filter>
</c:calendar-query>`;
    const res = await davFetch('REPORT', calPath, report, { depth: '1' });
    expect(res.status).toBe(207);
    const body = await res.text();
    expect(body).toContain('e2e-caldav-001');
    // 空窗口不含
    const empty = `<?xml version="1.0" encoding="utf-8"?>
<c:calendar-query xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">
  <d:prop><d:getetag/></d:prop>
  <c:filter><c:comp-filter name="VCALENDAR">
    <c:comp-filter name="VEVENT">
      <c:time-range start="20270101T000000Z" end="20270201T000000Z"/>
    </c:comp-filter>
  </c:comp-filter></c:filter>
</c:calendar-query>`;
    const res2 = await davFetch('REPORT', calPath, empty, { depth: '1' });
    expect(res2.status).toBe(207);
    expect(await res2.text()).not.toContain('e2e-caldav-001');
  });

  test('无认证 PROPFIND → 401 + WWW-Authenticate', async () => {
    const res = await fetch(`${BASE()}/dav/users/${user}/calendars/`, {
      method: 'PROPFIND',
      headers: { 'content-type': 'text/xml; charset=utf-8', depth: '0' },
      body: '<?xml version="1.0"?><d:propfind xmlns:d="DAV:"><d:prop><d:resourcetype/></d:prop></d:propfind>',
    });
    expect(res.status).toBe(401);
    expect(res.headers.get('www-authenticate')).toContain('Basic');
  });
});
