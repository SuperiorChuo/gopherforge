import { defineStore } from 'pinia';

import type { NotificationItem } from '@/types/interface';

const msgData = [
  {
    id: '123',
    content: '台球门店活动发布方案 已通过审核！',
    type: '内容动态',
    status: true,
    collected: false,
    date: '2021-01-01 08:00',
    quality: 'high',
  },
  {
    id: '124',
    content: 'MCN 达人直播排期 已同步成功！',
    type: '运营动态',
    status: true,
    collected: false,
    date: '2021-01-01 08:00',
    quality: 'low',
  },
  {
    id: '125',
    content: '2026-05-12 20:00的【台球直播复盘】会议即将开始，请提前10分钟前往 会议室1 进行签到！',
    type: '会议通知',
    status: true,
    collected: false,
    date: '2021-01-01 08:00',
    quality: 'middle',
  },
  {
    id: '126',
    content: '短视频素材批量发布任务 已完成！',
    type: '任务动态',
    status: true,
    collected: false,
    date: '2021-01-01 08:00',
    quality: 'low',
  },
];

type MsgDataType = typeof msgData;

export const useNotificationStore = defineStore('notification', {
  state: () => ({
    msgData,
  }),
  getters: {
    unreadMsg: (state) => state.msgData.filter((item: NotificationItem) => item.status),
    readMsg: (state) => state.msgData.filter((item: NotificationItem) => !item.status),
  },
  actions: {
    setMsgData(data: MsgDataType) {
      this.msgData = data;
    },
  },
  persist: true,
});
