// 极薄 fetch 封装（Node 原生），与 Playwright fixture 解耦，便于跨文件共享。
export const BASE = () => `http://127.0.0.1:${process.env.CALHUB_E2E_PORT}`;

export interface Result {
  status: number;
  data?: any;
  raw: string;
}

export async function api(method: string, path: string, body?: any, token?: string): Promise<Result> {
  const headers: Record<string, string> = {};
  if (body !== undefined) headers['content-type'] = 'application/json';
  if (token) headers['authorization'] = `Bearer ${token}`;
  const res = await fetch(BASE() + path, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  const raw = await res.text();
  let data: any;
  if (raw) {
    try { data = JSON.parse(raw); } catch { /* 非 JSON */ }
  }
  return { status: res.status, data, raw };
}
