# Page Development

Most pages in this repo are list pages. The recommended pattern is the **list-page trio**: `TableToolbar` (filters + actions) + a filter form + `Table`. Reference implementations: [`pages/system/dict`](https://github.com/SuperiorChuo/gopherforge/blob/main/microservices/web/src/pages/system/dict/index.tsx), [`pages/monitor/alerts`](https://github.com/SuperiorChuo/gopherforge/blob/main/microservices/web/src/pages/monitor/alerts/index.tsx).

## The List-Page Trio

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
    { title: 'Name', dataIndex: 'name' },
    // ...
    { title: 'Status', dataIndex: 'status', render: (v) => <StatusPill value={v} /> },
    { title: 'Actions', render: (_, r) => (
      <Space>
        {hasPerm('xxx:update') && <Button onClick={() => { setEditRecord(r); setModalOpen(true) }}>Edit</Button>}
        {hasPerm('xxx:delete') && <Button danger onClick={() => removeItem(r.id)}>Delete</Button>}
      </Space>
    ) },
  ]

  return (
    <div className="list-page">
      <TableToolbar
        preset="filter"
        formItems={[{ name: 'name', label: 'Name', el: <Input allowClear placeholder="Name" /> }]}
        onSearch={(values) => setParams({ ...params, page: 1, ...values })}
        actions={hasPerm('xxx:create') ? [<Button type="primary" onClick={() => { setEditRecord(null); setModalOpen(true) }}>New</Button>] : []}
      />
      <Table rowKey="id" columns={columns} dataSource={list} loading={loading}
        pagination={{ current: params.page, pageSize: params.page_size, total, showSizeChanger: true }}
        onChange={(pg) => setParams({ ...params, page: pg.current, page_size: pg.pageSize })} />
      <EditModal open={modalOpen} record={editRecord} onClose={() => setModalOpen(false)} onSaved={() => fetchList(params)} />
    </div>
  )
}
```

**State conventions** (consistent naming so teammates can pick up any page):

| State | Meaning |
|-------|---------|
| `list` / `total` | Table data & total count |
| `loading` | Fetching |
| `params` | Current filters + pagination (`{ page, page_size }`) |
| `modalOpen` / `editRecord` | Modal switch & edit target (`null` = create mode) |
| `submitting` | Form submitting (prevents double submit) |

**Data flow**: `setParams` triggers `useEffect` to refetch; filters reset to page 1 (`page: 1`).

## Pagination Details

- **Step back after delete**: if the current page becomes empty and it's not page 1, step back one page:

```ts
if (list.length === 1 && params.page > 1) {
  setParams({ ...params, page: params.page - 1 })
} else {
  fetchList(params)
}
```

- The table's `onChange` updates only `page` / `page_size`, keeping the filters intact.

## TableToolbar

`src/components/common/TableToolbar.tsx` standardises the filter + action area:

- `preset`: filter-form layout preset (see `ToolbarPreset` in the component).
- `formItems`: filter items (`name` / `label` / `el`).
- `onSearch(values)`: fires on "Search"; should `setParams({ ...params, page: 1, ...values })`.
- `actions`: right-side action buttons (New/Export…), wrap visibility in `hasPerm`.
- Also supports `onReset`, leading `children`, etc.

## Create / Edit Modal Form

```tsx
<Modal open={open} title={record ? 'Edit' : 'New'} onOk={submit} confirmLoading={submitting} onCancel={onClose}>
  <Form form={form} layout="vertical" initialValues={record ?? undefined}>
    <Form.Item name="name" label="Name" rules={[{ required: true, message: 'Please enter a name' }]}>
      <Input />
    </Form.Item>
  </Form>
</Modal>
```

- On open: create passes `record=null`, edit passes the record; use `useEffect([open, record])` to `form.setFieldsValue(record)` on open.
- On submit: call create/update, then `message.success` + close + refresh; failures are toasted by the request layer, nothing extra to handle.
- Use either `initialValues` or `setFieldsValue` for the edit state — not both.

## Shared Components (`src/components/`)

| Component | Purpose |
|-----------|---------|
| `TableToolbar` | Filter + action area for list pages |
| `StatusPill` | Status badge with tones (success/warning/danger/info) |
| `GlassEmpty` | Empty-state placeholder |
| `CountUpValue` | Count-up number animation (dashboard metrics) |
| `ExcelImportModal` | Excel import modal |
| `GeoMap` | Province/city geo distribution map (login logs) |
| `MonitorGaugeCard` | Monitoring gauge card (CPU/memory/disk) |
| `NotificationBell` | Header notification bell |
| `CommandPalette` | Global command palette (⌘K) |
| `ErrorBoundary` | Page error boundary |
| `Bpm*` (`BpmDynamicForm` / `BpmTaskActions` / `BpmInstanceTimeline` …) | Workflow-specific components |

## Theme & Style

- Wrap the page root with `className="list-page"` to inherit the unified layout in `list-pages.css`.
- Page styles must use **CSS variables** from `index.css` / `list-pages.css` (auto-adapt to both themes) — **don't hardcode colors**. See [Theme & Styling](/en/frontend/theme).

## Feedback

- Success toast: `import { message } from '@/utils/feedback'` → `message.success('Saved')` (not antd static methods).
- Confirmations: `modal.confirm(...)` (also from feedback).
