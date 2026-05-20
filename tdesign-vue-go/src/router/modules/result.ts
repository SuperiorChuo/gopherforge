import { LAYOUT } from '@/utils/route/constant';

export default [
  {
    path: '/result',
    name: 'result',
    component: LAYOUT,
    redirect: '/result/404',
    meta: {
      title: {
        zh_CN: '结果页',
      },
      icon: 'check-circle',
      hidden: true,
    },
    children: [
      {
        path: '403',
        name: 'Result403',
        component: () => import('@/pages/result/403/index.vue'),
        meta: { title: { zh_CN: '无权限' } },
      },
      {
        path: '404',
        name: 'Result404',
        component: () => import('@/pages/result/404/index.vue'),
        meta: { title: { zh_CN: '访问页面不存在页' } },
      },
      {
        path: '500',
        name: 'Result500',
        component: () => import('@/pages/result/500/index.vue'),
        meta: { title: { zh_CN: '服务器出错页' } },
      },
    ],
  },
];
