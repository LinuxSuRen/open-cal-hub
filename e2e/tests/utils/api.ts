// E2E 公共助手：API 封装、用户供给（bootstrap admin + outbox 邮箱验证）。
// 限流约束（注册 5/h/IP、登录 10/min/IP）决定了全轮复用少量用户：
// - rick：start-server.sh 以 CALHUB_AUTH_BOOTSTRAP_ADMIN_* 引导的管理员（已验证）
// - 其他用户按需 ensureUser 创建一次（走 outbox 验证邮件），跨文件复用
import { api, BASE, Result } from './fetch';

export { api, BASE };
export type { Result };

export const PASSWORD = 'passw0rd!';
export const ADMIN_USER = 'rick';

// login 登录并返回 access_token。
export async function login(username: string, password: string = PASSWORD): Promise<string> {
  const r = await api('POST', '/api/v1/auth/login', { email: `${username}@example.com`, password });
  if (r.status !== 200) {
    throw new Error(`login ${username}: ${r.status} ${r.raw}`);
  }
  return r.data.data.access_token as string;
}

// ensureAdmin 返回引导管理员（空库首启由 bootstrap 创建，已验证）令牌。
// 进程内缓存：登录限流 10/min/IP，全轮复用一次登录（TTL 15 分钟足够一轮）。
let adminTokenCache: string | undefined;
export async function ensureAdmin(): Promise<string> {
  if (adminTokenCache) return adminTokenCache;
  adminTokenCache = await login(ADMIN_USER);
  return adminTokenCache;
}

// userID 查用户 ID（登录后从 /me 取）。
export async function userID(token: string): Promise<number> {
  const r = await api('GET', '/api/v1/me', undefined, token);
  if (r.status !== 200) throw new Error(`/me: ${r.status} ${r.raw}`);
  return r.data.data.id as number;
}

// ensureUser 按需创建并验证用户（幂等：已注册则直接登录）。
// 验证令牌取自管理端邮件出箱（契约 §6/§9：outbox 模式）。
// 令牌进程内缓存（限流约束），注册全轮每用户仅一次。
const userTokenCache = new Map<string, string>();
export async function ensureUser(username: string): Promise<string> {
  const cached = userTokenCache.get(username);
  if (cached) return cached;
  const email = `${username}@example.com`;
  const reg = await api('POST', '/api/v1/auth/register', {
    email, password: PASSWORD, username,
  });
  if (reg.status === 409 || String(reg.raw).includes('email_taken')) {
    // 已存在：可能已验证（直接登录），也可能未验证（auth 组场景）——试登录判定
    const tryLogin = await api('POST', '/api/v1/auth/login', { email, password: PASSWORD });
    if (tryLogin.status === 200) {
      userTokenCache.set(username, tryLogin.data.data.access_token);
      return tryLogin.data.data.access_token as string;
    }
  } else if (reg.status !== 201) {
    throw new Error(`register ${username}: ${reg.status} ${reg.raw}`);
  }
  // 新注册（或存在但未验证）：从管理端出箱取验证令牌完成验证
  {
    const admin = await ensureAdmin();
    const outbox = await api('GET', '/api/v1/admin/emails?page=1&page_size=50', undefined, admin);
    if (outbox.status !== 200) throw new Error(`admin emails: ${outbox.status} ${outbox.raw}`);
    const hit = (outbox.data.data as any[]).find((m) => m.to === email && m.kind === 'verify_email');
    if (!hit) throw new Error(`verify email for ${email} not found in outbox`);
    const m = String(hit.body_text).match(/token=([A-Za-z0-9_\-]+)/);
    if (!m) throw new Error(`token not in mail body: ${hit.body_text}`);
    const verify = await api('POST', '/api/v1/auth/verify-email', { token: m[1] });
    if (verify.status !== 200) throw new Error(`verify-email: ${verify.status} ${verify.raw}`);
  }
  const token = await login(username);
  userTokenCache.set(username, token);
  return token;
}

// firstMondaySlot 从 slots 接口取未来第一个可用槽位开始时刻。
export async function firstSlot(username: string, slug: string, skip: Set<string> = new Set()): Promise<string> {
  const start = new Date().toISOString().slice(0, 10);
  const r = await api('GET', `/api/v1/pub/u/${username}/${slug}/slots?start=${start}&days=28&tz=Asia/Shanghai`);
  if (r.status !== 200) throw new Error(`slots: ${r.status} ${r.raw}`);
  for (const day of r.data.data.days as any[]) {
    for (const s of day.slots as any[]) {
      if (!skip.has(s.start)) return s.start as string;
    }
  }
  throw new Error('no free slot within 28 days');
}

// setupHost 建可用时间集（周一 09:00-18:00 上海）与 60 分钟预约类型（幂等）。
export async function setupHost(username: string): Promise<string> {
  const token = await ensureUser(username);
  // intro 类型已存在则复用（user_id+slug 唯一，重复创建会 400）
  const existing = await api('GET', '/api/v1/event-types', undefined, token);
  if (existing.status === 200 &&
      (existing.data.data as any[]).some((t) => t.slug === 'intro')) {
    return token;
  }
  const avail = await api('POST', '/api/v1/availability-sets', {
    name: '工作时间', timezone: 'Asia/Shanghai', is_default: true,
    rules: [{ weekday: 1, start_minute: 540, end_minute: 1080 }],
  }, token);
  if (avail.status !== 201) throw new Error(`availability: ${avail.status} ${avail.raw}`);
  const et = await api('POST', '/api/v1/event-types', {
    slug: 'intro', name: '介绍会谈', duration_minutes: 60,
    availability_id: avail.data.data.id, min_notice_minutes: 0,
  }, token);
  if (et.status !== 201) throw new Error(`event-type: ${et.status} ${et.raw}`);
  return token;
}

// nextMonday 返回下一个周一（UTC）的 YYYY-MM-DD。
export function nextMonday(): string {
  const d = new Date();
  const add = (8 - d.getUTCDay()) % 7 || 7; // 周一=1；今天周一则下周一
  d.setUTCDate(d.getUTCDate() + add);
  return d.toISOString().slice(0, 10);
}
