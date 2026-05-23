import { defineStore } from 'pinia';

import { createNotificationTicket } from '@/api/auth';
import type { NotificationItem } from '@/types/interface';

interface RealtimeNotificationPayload {
  id?: string;
  type?: string;
  title?: string;
  content?: string;
  created_at?: string;
  link?: string;
}

let notificationSocket: WebSocket | null = null;
let notificationConnectPromise: Promise<void> | null = null;
let notificationReconnectTimer: ReturnType<typeof setTimeout> | null = null;
let notificationToken = '';
let shouldReconnectNotificationSocket = false;

const NOTIFICATION_RECONNECT_DELAY = 300;

function clearNotificationReconnectTimer() {
  if (notificationReconnectTimer) {
    clearTimeout(notificationReconnectTimer);
    notificationReconnectTimer = null;
  }
}

function realtimeTypeLabel(type?: string) {
  switch (type) {
    case 'announcement':
      return '系统公告';
    case 'job_alert':
      return '任务告警';
    case 'approval':
      return '审批提醒';
    default:
      return '系统通知';
  }
}

function buildNotificationContent(payload: RealtimeNotificationPayload) {
  const title = payload.title?.trim();
  const content = payload.content?.trim();
  if (title && content) return `${title}: ${content}`;
  return title || content || '收到新的系统通知';
}

function buildNotificationWebSocketURL(ticket: string) {
  const apiPrefix = import.meta.env.VITE_API_URL_PREFIX || '/api/v1';
  const apiHost =
    import.meta.env.VITE_IS_REQUEST_PROXY === 'true' && import.meta.env.VITE_API_URL
      ? import.meta.env.VITE_API_URL
      : window.location.origin;
  const url = new URL(`${apiPrefix}/ws/notifications`, apiHost);
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  url.searchParams.set('ticket', ticket);
  return url.toString();
}

function toNotificationItem(payload: RealtimeNotificationPayload): NotificationItem {
  return {
    id: payload.id || `notification:${Date.now()}`,
    content: buildNotificationContent(payload),
    type: realtimeTypeLabel(payload.type),
    status: true,
    collected: false,
    date: payload.created_at || new Date().toISOString(),
    quality: payload.type === 'job_alert' ? 'high' : 'middle',
  };
}

export const useNotificationStore = defineStore('notification', {
  state: () => ({
    msgData: [] as NotificationItem[],
  }),
  getters: {
    unreadMsg: (state) => state.msgData.filter((item: NotificationItem) => item.status),
    readMsg: (state) => state.msgData.filter((item: NotificationItem) => !item.status),
  },
  actions: {
    setMsgData(data: NotificationItem[]) {
      this.msgData = data;
    },
    addRealtimeMessage(payload: RealtimeNotificationPayload) {
      const item = toNotificationItem(payload);
      this.msgData = [item, ...this.msgData.filter((message) => message.id !== item.id)].slice(0, 50);
    },
    scheduleReconnect() {
      if (!shouldReconnectNotificationSocket || !notificationToken || notificationReconnectTimer) return;

      notificationReconnectTimer = setTimeout(() => {
        notificationReconnectTimer = null;
        if (shouldReconnectNotificationSocket && notificationToken) {
          void this.connect(notificationToken);
        }
      }, NOTIFICATION_RECONNECT_DELAY);
    },
    async connect(token: string) {
      if (!token || typeof WebSocket === 'undefined') return;
      notificationToken = token;
      shouldReconnectNotificationSocket = true;
      clearNotificationReconnectTimer();

      if (notificationSocket && notificationSocket.readyState <= WebSocket.OPEN) return;
      if (notificationConnectPromise) return notificationConnectPromise;

      notificationConnectPromise = (async () => {
        try {
          const { ticket } = await createNotificationTicket();
          if (!shouldReconnectNotificationSocket || token !== notificationToken) return;
          if (!ticket) {
            this.scheduleReconnect();
            return;
          }

          const socket = new WebSocket(buildNotificationWebSocketURL(ticket));
          notificationSocket = socket;
          socket.onmessage = (event) => {
            try {
              this.addRealtimeMessage(JSON.parse(event.data));
            } catch {
              this.addRealtimeMessage({ content: String(event.data || '') });
            }
          };
          socket.onclose = () => {
            if (notificationSocket === socket) {
              notificationSocket = null;
              this.scheduleReconnect();
            }
          };
        } catch {
          notificationSocket = null;
          this.scheduleReconnect();
        } finally {
          notificationConnectPromise = null;
        }
      })();
      return notificationConnectPromise;
    },
    disconnect() {
      shouldReconnectNotificationSocket = false;
      notificationToken = '';
      clearNotificationReconnectTimer();
      notificationSocket?.close();
      notificationSocket = null;
    },
  },
  persist: {
    key: 'notification',
    pick: ['msgData'],
  },
});
