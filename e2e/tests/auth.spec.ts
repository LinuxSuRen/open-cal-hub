// 组 1：注册（outbox 验证邮件）→ 验证 → 登录。
import { test, expect } from '@playwright/test';
import { api, ensureAdmin, ensureUser, PASSWORD } from './utils/api';

test.describe.serial('认证流程', () => {
  test('注册 → 未验证登录被拒 → outbox 验证 → 登录成功', async () => {
    // dave：本组一次性用户（注册后先体验"未验证"状态再完成验证）
    const reg = await api('POST', '/api/v1/auth/register', {
      email: 'dave@example.com', password: PASSWORD, username: 'dave',
    });
    expect(reg.status).toBe(201);
    expect(reg.data.data.verification_required).toBe(true);

    // 未验证登录被拒
    const early = await api('POST', '/api/v1/auth/login', { email: 'dave@example.com', password: PASSWORD });
    expect(early.status).toBe(403);
    expect(early.data.error.code).toBe('auth/email_not_verified');

    // 从管理端出箱取验证链接完成验证（ensureUser 内部流程，此处 dave 已注册 → 走验证+登录）
    const token = await ensureUser('dave');
    expect(token).toBeTruthy();

    // 登录后 /me 可用
    const me = await api('GET', '/api/v1/me', undefined, token);
    expect(me.status).toBe(200);
    expect(me.data.data.email_verified).toBe(true);
  });

  test('错误密码登录 → 401 invalid_credentials', async () => {
    const r = await api('POST', '/api/v1/auth/login', { email: 'dave@example.com', password: 'wrong1pass' });
    expect(r.status).toBe(401);
    expect(r.data.error.code).toBe('auth/invalid_credentials');
  });

  test('重复注册同邮箱 → 409 email_taken', async () => {
    const r = await api('POST', '/api/v1/auth/register', {
      email: 'dave@example.com', password: PASSWORD, username: 'dave2',
    });
    expect(r.status).toBe(409);
    expect(r.data.error.code).toBe('auth/email_taken');
  });

  test('管理端可登录（顺带锁定 bootstrap）', async () => {
    const admin = await ensureAdmin();
    expect(admin).toBeTruthy();
  });
});
