# 页面开发规范

本仓库绝大多数页面是「列表页」。推荐范式统一为**列表页三件套**：`TableToolbar`（筛选 + 动作）+ 过滤表单 + `Table`。参照范例：[`pages/system/dict`](https://github.com/SuperiorChuo/gopherforge/blob/main/microservices/web/src/pages/system/dict/index.tsx)、[`pages/monitor/alerts`](https://github.com/SuperiorChuo/gopherforge/blob/main/microservices/web/src/pages/monitor/alerts/index.tsx)。

## 列表页三件套

```tsx
export default function ItemList() {
  const [list, setList] = useState<Item[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useState<PageParams>({ page: 1, page_size: 10 })
  const [modalOpen, setModalOpen] = useState(false)
  const [editRecord, setEditRecord] = useState<Item | null>(null)
  const { hasPerm } = usePermission()

  const fetchList = useCallback(async (p: PageParams) => {
    setLoading(true)
    try {
      const data = await getList(p)
      setList(data.list)
      setTotal(data.total)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { void fetchList(params) }, [params, fetchList])

  const columns: ColumnsType<Item> = [
    { title: '名称', dataIndex: 'name' },
    // ...
    { title: '状态', dataIndex: 'status', render: (v) => <StatusPill value={v} /> },
    { title: '操作', render: (_, r) => (
      <Space>
        {hasPerm('xxx:update') && <Button onClick={() => { setEditRecord(r); setModalOpen(true) }}>编辑</Button>}
        {hasPerm('xxx:delete') && <Button danger onClick={() => removeItem(r.id)}>删除</Button>}
      </Space>
    ) },
  ]

  return (
    <div className="list-page">
      <TableToolbar
        preset="filter"                          // 筛选区预设（见下）
        formItems={[{ name: 'name', label: '名称', el: <Input allowClear placeholder="名称" /> }]}
        onSearch={(values) => setParams({ ...params, page: 1, ...values })}
        actions={hasPerm('xxx:create') ? [<Button type="primary" onClick={() => { setEditRecord(null); setModalOpen(true) }}>新增</Button>] : []}
      />
      <Table rowKey="id" columns={columns} dataSource={list} loading={loading}
        pagination={{ current: params.page, pageSize: params.page_size, total, showSizeChanger: true }}
        onChange={(pg) => setParams({ ...params, page: pg.current, page_size: pg.pageSize })} />
      <EditModal open={modalOpen} record={editRecord} onClose={() => setModalOpen(false)} onSaved={() => fetchList(params)} />
    </div>
  )
}
```

**状态约定**（统一命名，方便同事接手）：

| 状态 | 含义 |
|------|------|
| `list` / `total` | 表格数据与总数 |
| `loading` | 加载中 |
| `params` | 当前查询条件 + 分页（`{ page, page_size }`） |
| `modalOpen` / `editRecord` | 弹窗开关与编辑对象（`null` = 新增模式） |
| `submitting` | 提交中（防重复提交） |

**数据流**：`setParams` 触发 `useEffect` 重新拉取；筛选重置到第 1 页（`page: 1`）。

## 分页交互细节

- **删除后回退**：若删完当前页为空且非第 1 页，自动回退一页再拉取：

```ts
if (list.length === 1 && params.page > 1) {
  setParams({ ...params, page: params.page - 1 })
} else {
  fetchList(params)
}
```

- 表格 `onChange` 只更新 `page` / `page_size`，不丢筛选条件。

## TableToolbar

`src/components/common/TableToolbar.tsx` 统一了筛选区与操作区，减少重复：

- `preset`：筛选表单布局预设（`filter` 等，见组件内 `ToolbarPreset`）。
- `formItems`：筛选表单项数组（name / label / el）。
- `onSearch(values)`：点击「查询」回调，应 `setParams({ ...params, page: 1, ...values })`。
- `actions`：右侧操作按钮（新增/导出等，记得用 `hasPerm` 包可见性）。
- 还支持 `onReset`、左侧额外 `children` 等。

## 新增 / 编辑弹窗表单

```tsx
<Modal open={open} title={record ? '编辑' : '新增'} onOk={submit} confirmLoading={submitting} onCancel={onClose}>
  <Form form={form} layout="vertical" initialValues={record ?? undefined}>
    <Form.Item name="name" label="名称" rules={[{ required: true, message: '请输入名称' }]}>
      <Input />
    </Form.Item>
  </Form>
</Modal>
```

- 打开时：新增 `record=null`，编辑 `record=对象`；用 `useEffect([open, record])` 在打开瞬间 `form.setFieldsValue(record)`。
- 提交：调 `create/update`，成功后 `message.success` + 关闭 + 刷新列表；失败由请求层统一弹错，不额外处理。
- 编辑态 `initialValues` 或 `setFieldsValue` 二选一，不要两处都设。

## 公共组件清单（`src/components/`）

| 组件 | 用途 |
|------|------|
| `TableToolbar` | 列表页筛选区 + 操作区 |
| `StatusPill` | 状态徽章（自带 tone：success/warning/danger/info，如 `正常/等待确认/告警中`） |
| `GlassEmpty` | 空状态占位（玻璃质感） |
| `CountUpValue` | 数字滚动动画（仪表盘指标） |
| `ExcelImportModal` | Excel 导入弹窗 |
| `GeoMap` | 省市地理分布图（登录日志） |
| `MonitorGaugeCard` | 监控仪表盘卡片（CPU/内存/磁盘） |
| `NotificationBell` | 顶栏通知铃铛 |
| `CommandPalette` | 全局命令面板（⌘K） |
| `ErrorBoundary` | 页面错误边界 |
| `Bpm*`（`BpmDynamicForm` / `BpmTaskActions` / `BpmInstanceTimeline` …） | 审批流专用组件 |

## 主题与样式约定

- 页面根元素建议加 `className="list-page"` 等统一类，样式走 `index.css` / `list-pages.css` 里的 **CSS 变量**（双主题自动适配），**不要写死颜色值**。
- 详见[主题与样式](/frontend/theme)。

## 反馈

- 成功提示：`import { message } from '@/utils/feedback'` → `message.success('已保存')`（不要用 antd 静态方法）。
- 二次确认：`modal.confirm(...)`（同上从 feedback 引入）。
