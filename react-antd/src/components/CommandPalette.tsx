import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Modal, Input, Empty } from 'antd'
import { SearchOutlined, EnterOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import type { InputRef } from 'antd'

export interface PaletteItem {
  label: string
  path: string
  group: string
  icon?: React.ReactNode
}

const RECENT_STORAGE_KEY = 'command_palette_recent'

function loadRecent(): string[] {
  try {
    return JSON.parse(localStorage.getItem(RECENT_STORAGE_KEY) ?? '[]')
  } catch {
    return []
  }
}

function saveRecent(path: string) {
  const list = [path, ...loadRecent().filter((p) => p !== path)].slice(0, 5)
  localStorage.setItem(RECENT_STORAGE_KEY, JSON.stringify(list))
}

export default function CommandPalette({ items }: { items: PaletteItem[] }) {
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const inputRef = useRef<InputRef>(null)

  // 全局 ⌘K / Ctrl+K 唤起
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        setOpen((v) => !v)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  useEffect(() => {
    if (open) {
      setQuery('')
      setActive(0)
      // Modal 渲染后再聚焦
      setTimeout(() => inputRef.current?.focus(), 50)
    }
  }, [open])

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) {
      // 无输入时：最近访问置顶，其余按分组排列
      const recent = loadRecent()
      const byRecent = (a: PaletteItem, b: PaletteItem) => {
        const ia = recent.indexOf(a.path)
        const ib = recent.indexOf(b.path)
        return (ia === -1 ? 99 : ia) - (ib === -1 ? 99 : ib)
      }
      return [...items].sort(byRecent)
    }
    return items.filter(
      (it) =>
        it.label.toLowerCase().includes(q) ||
        it.path.toLowerCase().includes(q) ||
        it.group.toLowerCase().includes(q),
    )
  }, [items, query])

  const go = useCallback(
    (item: PaletteItem) => {
      saveRecent(item.path)
      setOpen(false)
      navigate(item.path)
    },
    [navigate],
  )

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setActive((a) => Math.min(a + 1, filtered.length - 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setActive((a) => Math.max(a - 1, 0))
    } else if (e.key === 'Enter' && filtered[active]) {
      e.preventDefault()
      go(filtered[active])
    }
  }

  const recentPaths = loadRecent()

  return (
    <Modal
      open={open}
      onCancel={() => setOpen(false)}
      footer={null}
      closable={false}
      width={560}
      style={{ top: 120 }}
      styles={{ body: { padding: 0 } }}
      className="cmdk-modal"
      destroyOnHidden
    >
      <div className="cmdk-input-wrap">
        <Input
          ref={inputRef}
          size="large"
          variant="borderless"
          prefix={<SearchOutlined style={{ color: 'rgba(148, 163, 184, 0.6)' }} />}
          placeholder="搜索页面，↑↓ 选择，回车跳转…"
          value={query}
          onChange={(e) => {
            setQuery(e.target.value)
            setActive(0)
          }}
          onKeyDown={onKeyDown}
        />
      </div>
      <div className="cmdk-list">
        {filtered.length === 0 ? (
          <Empty description="没有匹配的页面" image={Empty.PRESENTED_IMAGE_SIMPLE} style={{ padding: '32px 0' }} />
        ) : (
          filtered.map((it, i) => (
            <div
              key={it.path}
              className={`cmdk-item${i === active ? ' cmdk-item-active' : ''}`}
              onMouseEnter={() => setActive(i)}
              onClick={() => go(it)}
            >
              <span className="cmdk-item-icon">{it.icon}</span>
              <span className="cmdk-item-label">{it.label}</span>
              <span className="cmdk-item-group">
                {!query && recentPaths.includes(it.path) ? '最近访问' : it.group}
              </span>
              {i === active && <EnterOutlined className="cmdk-item-enter" />}
            </div>
          ))
        )}
      </div>
      <div className="cmdk-foot">
        <span><kbd>↑</kbd><kbd>↓</kbd> 选择</span>
        <span><kbd>↵</kbd> 跳转</span>
        <span><kbd>Esc</kbd> 关闭</span>
      </div>
    </Modal>
  )
}
