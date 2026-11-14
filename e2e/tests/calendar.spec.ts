// 组 2：日历——建日历 → 单次事件 → 每周重复 → 展开 → 改单实例 → 删整个系列。
import { test, expect } from '@playwright/test';
import { api, ensureUser, nextMonday } from './utils/api';

test.describe.serial('日历与事件', () => {
  const user = 'dave'; // 全轮共享用户（限流 5/h/IP，见 utils/api.ts）
  let calID: number;
  let token: string;

  test('注册钩子已建默认日历；单次事件写入', async () => {
    token = await ensureUser(user);
    const list = await api('GET', '/api/v1/calendars', undefined, token);
    expect(list.status).toBe(200);
    const own = list.data.data as any[];
    const def = own.find((c) => c.is_default);
    expect(def).toBeTruthy(); // 注册钩子创建的默认日历
    calID = def.id;

    const ev = await api('POST', `/api/v1/calendars/${calID}/events`, {
      summary: '单次会议', start: '2026-12-08T02:00:00Z', end: '2026-12-08T03:00:00Z', tz: 'UTC',
    }, token);
    expect(ev.status).toBe(201);

    // 注：dave 为全轮共享用户，其默认日历中可能已有 CalDAV 组等写入的事件，
    // 故按 summary 断言存在性而非总数。
    const events = await api('GET', `/api/v1/calendars/${calID}/events?from=2026-12-01T00:00:00Z&to=2026-12-31T00:00:00Z`, undefined, token);
    expect(events.status).toBe(200);
    expect((events.data.data as any[]).some((i) => i.summary === '单次会议')).toBe(true);
  });

  test('每周重复事件按周展开', async () => {
    const monday = nextMonday();
    const start = `${monday}T09:00:00Z`;
    const ev = await api('POST', `/api/v1/calendars/${calID}/events`, {
      summary: '每周例会', start, end: `${monday}T10:00:00Z`, tz: 'UTC',
      rrule: { freq: 'WEEKLY', byday: ['MO'] },
    }, token);
    expect(ev.status).toBe(201);
    const masterID = ev.data.data.id;

    // 展开四周 ≥4 个实例，且带 recurrence_id（scope=single 定位依据）
    const from = `${monday}T00:00:00Z`;
    const to = new Date(new Date(from).getTime() + 28 * 86400_000).toISOString().replace(/\.\d+Z$/, 'Z');
    const list = await api('GET', `/api/v1/calendars/${calID}/events?from=${from}&to=${to}`, undefined, token);
    const all = list.data.data as any[];
    const insts = all.filter((i) => i.summary === '每周例会');
    expect(insts.length).toBeGreaterThanOrEqual(4);
    expect(insts.every((i) => i.recurring)).toBe(true);

    // 改第二个实例（scope=single）：仅该实例标题变化
    const target = insts[1];
    expect(target.recurrence_id).toBeTruthy();
    const patch = await api('PATCH', `/api/v1/events/${masterID}?scope=single`, {
      summary: '改期例会', recurrence_id: target.recurrence_id,
    }, token);
    expect(patch.status).toBe(200);

    const after = await api('GET', `/api/v1/calendars/${calID}/events?from=${from}&to=${to}`, undefined, token);
    const afterInsts = after.data.data as any[];
    const changed = afterInsts.filter((i) => i.summary === '改期例会');
    expect(changed.length).toBe(1);
    expect(changed[0].start).toBe(target.start);

    // 删整个系列（含覆盖行）：原展开窗口不再出现该系列（窗口 ≤62 天）
    const del = await api('DELETE', `/api/v1/events/${masterID}?scope=all`, undefined, token);
    expect(del.status).toBe(200);
    const final = await api('GET', `/api/v1/calendars/${calID}/events?from=${from}&to=${to}`, undefined, token);
    const rest = final.data.data as any[];
    expect(rest.filter((i) => i.summary === '每周例会' || i.summary === '改期例会').length).toBe(0);
  });
});
