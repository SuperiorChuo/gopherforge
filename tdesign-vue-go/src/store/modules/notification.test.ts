import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';

vi.mock('@/api/auth', () => ({
  createNotificationTicket: vi.fn(async () => ({ ticket: 'test-ticket' })),
}));

import { createNotificationTicket } from '@/api/auth';

import { useNotificationStore } from './notification';

class MockWebSocket {
  static CONNECTING = 0;

  static OPEN = 1;

  static CLOSING = 2;

  static CLOSED = 3;

  static instances: MockWebSocket[] = [];

  readyState = MockWebSocket.OPEN;

  onclose: (() => void) | null = null;

  onmessage: ((event: { data: string }) => void) | null = null;

  constructor(public url: string) {
    MockWebSocket.instances.push(this);
  }

  close() {
    this.readyState = MockWebSocket.CLOSED;
    this.onclose?.();
  }

  serverClose() {
    this.close();
  }
}

describe('notification store websocket payloads', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.useFakeTimers();
    vi.mocked(createNotificationTicket).mockReset();
    vi.mocked(createNotificationTicket).mockResolvedValue({ ticket: 'test-ticket' });
    MockWebSocket.instances = [];
    vi.stubGlobal('WebSocket', MockWebSocket);
    vi.stubGlobal('window', { location: { origin: 'http://localhost' } });
  });

  afterEach(() => {
    useNotificationStore().disconnect();
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('prepends unread websocket messages and maps announcement type', () => {
    const store = useNotificationStore();

    store.addRealtimeMessage({
      id: 'notice:42',
      type: 'announcement',
      title: '系统维护',
      content: '今晚 23:00 发布新版本',
      created_at: '2026-05-22T12:00:00Z',
    });

    expect(store.unreadMsg).toHaveLength(1);
    expect(store.unreadMsg[0]).toMatchObject({
      id: 'notice:42',
      content: '系统维护: 今晚 23:00 发布新版本',
      type: '系统公告',
      status: true,
    });
  });

  it('requests a fresh ticket and reconnects after the websocket closes', async () => {
    vi.mocked(createNotificationTicket)
      .mockResolvedValueOnce({ ticket: 'ticket-1' })
      .mockResolvedValueOnce({ ticket: 'ticket-2' });
    const store = useNotificationStore();

    await store.connect('token');
    MockWebSocket.instances[0].serverClose();
    await vi.advanceTimersByTimeAsync(300);

    expect(createNotificationTicket).toHaveBeenCalledTimes(2);
    expect(MockWebSocket.instances).toHaveLength(2);
    expect(MockWebSocket.instances[1].url).toContain('ticket=ticket-2');
  });

  it('does not reconnect after disconnect closes the websocket', async () => {
    vi.mocked(createNotificationTicket).mockResolvedValueOnce({ ticket: 'ticket-1' });
    const store = useNotificationStore();

    await store.connect('token');
    store.disconnect();
    await vi.advanceTimersByTimeAsync(300);

    expect(createNotificationTicket).toHaveBeenCalledTimes(1);
    expect(MockWebSocket.instances).toHaveLength(1);
  });
});
