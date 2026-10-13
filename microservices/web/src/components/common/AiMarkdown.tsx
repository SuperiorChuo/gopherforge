import { Fragment, type ReactNode } from 'react'

/**
 * 极简 Markdown 渲染（零依赖）：支持代码块/行内代码/粗体/斜体/链接/
 * 标题/无序有序列表/引用/分隔线，其余按段落 + 换行保底展示。
 * 全程构建 React 元素，不走 innerHTML，天然防注入。
 */

// 行内样式：`code`、**bold**、*em*、[text](url)
function renderInline(text: string, keyPrefix: string): ReactNode[] {
  const nodes: ReactNode[] = []
  const pattern = /(`[^`]+`)|(\*\*[^*]+\*\*)|(\*[^*\n]+\*)|(\[[^\]]+\]\((https?:\/\/[^\s)]+)\))/g
  let last = 0
  let m: RegExpExecArray | null
  let i = 0
  while ((m = pattern.exec(text)) !== null) {
    if (m.index > last) nodes.push(text.slice(last, m.index))
    const token = m[0]
    if (token.startsWith('`')) {
      nodes.push(<code key={`${keyPrefix}-c${i}`} className="ai-md-code">{token.slice(1, -1)}</code>)
    } else if (token.startsWith('**')) {
      nodes.push(<strong key={`${keyPrefix}-b${i}`}>{token.slice(2, -2)}</strong>)
    } else if (token.startsWith('*')) {
      nodes.push(<em key={`${keyPrefix}-i${i}`}>{token.slice(1, -1)}</em>)
    } else {
      const label = token.slice(1, token.indexOf(']'))
      nodes.push(
        <a key={`${keyPrefix}-a${i}`} href={m[5]} target="_blank" rel="noopener noreferrer">
          {label}
        </a>,
      )
    }
    last = m.index + token.length
    i += 1
  }
  if (last < text.length) nodes.push(text.slice(last))
  return nodes
}

interface Block {
  type: 'code' | 'heading' | 'ul' | 'ol' | 'quote' | 'hr' | 'table' | 'p'
  lines: string[]
  level?: number
  lang?: string
  header?: string[]
  rows?: string[][]
}

// 表格行切分：剥掉首尾竖线后按 | 切分，各单元去空白
function splitTableRow(line: string): string[] {
  let s = line.trim()
  if (s.startsWith('|')) s = s.slice(1)
  if (s.endsWith('|')) s = s.slice(0, -1)
  return s.split('|').map((c) => c.trim())
}

// 分隔行判定：必须含竖线（排除裸 --- 分隔线），每个单元是 :?-+:? 形态
function isTableDelimiter(line: string): boolean {
  if (!line.includes('|')) return false
  const cells = splitTableRow(line)
  return cells.length >= 1 && cells.every((c) => /^:?-{1,}:?$/.test(c))
}

function parseBlocks(md: string): Block[] {
  const lines = md.replace(/\r\n/g, '\n').split('\n')
  const blocks: Block[] = []
  let idx = 0
  while (idx < lines.length) {
    const line = lines[idx]
    // 代码块：任何以 ``` 开头的行都按围栏处理（LLM 常写 "``` go"、"```sql -- x"），
    // 必须与下方段落守卫的 /^```/ 完全一致，否则该行两边都不认领 → idx 不前进死循环
    const fence = line.match(/^```\s*(.*)$/)
    if (fence) {
      const code: string[] = []
      idx += 1
      while (idx < lines.length && !/^```/.test(lines[idx])) {
        code.push(lines[idx])
        idx += 1
      }
      idx += 1 // 跳过闭合 ```（流式渲染时可能还没到，也照常收尾）
      blocks.push({ type: 'code', lines: code, lang: fence[1].trim().split(/\s+/)[0] || undefined })
      continue
    }
    // 空行
    if (!line.trim()) {
      idx += 1
      continue
    }
    // 标题
    const heading = line.match(/^(#{1,6})\s+(.*)$/)
    if (heading) {
      blocks.push({ type: 'heading', level: heading[1].length, lines: [heading[2]] })
      idx += 1
      continue
    }
    // 分隔线
    if (/^\s*(-{3,}|\*{3,})\s*$/.test(line)) {
      blocks.push({ type: 'hr', lines: [] })
      idx += 1
      continue
    }
    // 引用
    if (/^\s*>\s?/.test(line)) {
      const quote: string[] = []
      while (idx < lines.length && /^\s*>\s?/.test(lines[idx])) {
        quote.push(lines[idx].replace(/^\s*>\s?/, ''))
        idx += 1
      }
      blocks.push({ type: 'quote', lines: quote })
      continue
    }
    // 无序列表
    if (/^\s*[-*+]\s+/.test(line)) {
      const items: string[] = []
      while (idx < lines.length && /^\s*[-*+]\s+/.test(lines[idx])) {
        items.push(lines[idx].replace(/^\s*[-*+]\s+/, ''))
        idx += 1
      }
      blocks.push({ type: 'ul', lines: items })
      continue
    }
    // 有序列表
    if (/^\s*\d+[.)]\s+/.test(line)) {
      const items: string[] = []
      while (idx < lines.length && /^\s*\d+[.)]\s+/.test(lines[idx])) {
        items.push(lines[idx].replace(/^\s*\d+[.)]\s+/, ''))
        idx += 1
      }
      blocks.push({ type: 'ol', lines: items })
      continue
    }
    // 表格：含竖线的表头行 + 紧随的分隔行（LLM 极常输出 | 列 | 列 |），
    // 否则整段会落到段落分支显示成字面管道符
    if (line.includes('|') && idx + 1 < lines.length && isTableDelimiter(lines[idx + 1])) {
      const header = splitTableRow(line)
      idx += 2 // 跳过表头行和分隔行
      const rows: string[][] = []
      while (idx < lines.length && lines[idx].trim() && lines[idx].includes('|')) {
        rows.push(splitTableRow(lines[idx]))
        idx += 1
      }
      blocks.push({ type: 'table', lines: [], header, rows })
      continue
    }
    // 段落：吃到下一个空行/块级标记
    const para: string[] = []
    while (
      idx < lines.length &&
      lines[idx].trim() &&
      !/^```/.test(lines[idx]) &&
      !/^(#{1,6})\s+/.test(lines[idx]) &&
      !/^\s*[-*+]\s+/.test(lines[idx]) &&
      !/^\s*\d+[.)]\s+/.test(lines[idx]) &&
      !/^\s*>\s?/.test(lines[idx]) &&
      !/^\s*(-{3,}|\*{3,})\s*$/.test(lines[idx]) &&
      // 表头行（下一行是分隔行）交给表格分支，别并进段落
      !(lines[idx].includes('|') && idx + 1 < lines.length && isTableDelimiter(lines[idx + 1]))
    ) {
      para.push(lines[idx])
      idx += 1
    }
    if (para.length === 0) {
      // 兜底：当前行没被任何分支认领时按纯文本吃掉，保证 idx 永远前进
      para.push(lines[idx])
      idx += 1
    }
    blocks.push({ type: 'p', lines: para })
  }
  return blocks
}

export default function AiMarkdown({ content }: { content: string }) {
  const blocks = parseBlocks(content)
  return (
    <div className="ai-md">
      {blocks.map((block, bi) => {
        const key = `blk-${bi}`
        switch (block.type) {
          case 'code':
            return (
              <pre key={key} className="ai-md-pre glass-well">
                {block.lang && <span className="ai-md-lang">{block.lang}</span>}
                <code>{block.lines.join('\n')}</code>
              </pre>
            )
          case 'heading': {
            const level = Math.min(block.level ?? 1, 6)
            // 收窄到实际会取到的六个标题标签：@react-three/fiber 会把 three 元素
            // 增广进 JSX.IntrinsicElements，宽泛断言会让 children 被压成 never
            const Tag = `h${level}` as 'h1' | 'h2' | 'h3' | 'h4' | 'h5' | 'h6'
            return <Tag key={key} className="ai-md-heading">{renderInline(block.lines[0], key)}</Tag>
          }
          case 'ul':
            return (
              <ul key={key} className="ai-md-list">
                {block.lines.map((item, li) => (
                  <li key={`${key}-${li}`}>{renderInline(item, `${key}-${li}`)}</li>
                ))}
              </ul>
            )
          case 'ol':
            return (
              <ol key={key} className="ai-md-list">
                {block.lines.map((item, li) => (
                  <li key={`${key}-${li}`}>{renderInline(item, `${key}-${li}`)}</li>
                ))}
              </ol>
            )
          case 'quote':
            return (
              <blockquote key={key} className="ai-md-quote">
                {block.lines.map((l, li) => (
                  <Fragment key={`${key}-${li}`}>
                    {renderInline(l, `${key}-${li}`)}
                    {li < block.lines.length - 1 && <br />}
                  </Fragment>
                ))}
              </blockquote>
            )
          case 'hr':
            return <hr key={key} className="ai-md-hr" />
          case 'table':
            return (
              <div key={key} className="ai-md-table-wrap">
                <table className="ai-md-table">
                  {block.header && block.header.length > 0 && (
                    <thead>
                      <tr>
                        {block.header.map((h, hi) => (
                          <th key={`${key}-h${hi}`}>{renderInline(h, `${key}-h${hi}`)}</th>
                        ))}
                      </tr>
                    </thead>
                  )}
                  <tbody>
                    {(block.rows ?? []).map((row, ri) => (
                      <tr key={`${key}-r${ri}`}>
                        {row.map((cell, ci) => (
                          <td key={`${key}-r${ri}-c${ci}`}>{renderInline(cell, `${key}-r${ri}-c${ci}`)}</td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )
          default:
            return (
              <p key={key} className="ai-md-p">
                {block.lines.map((l, li) => (
                  <Fragment key={`${key}-${li}`}>
                    {renderInline(l, `${key}-${li}`)}
                    {li < block.lines.length - 1 && <br />}
                  </Fragment>
                ))}
              </p>
            )
        }
      })}
    </div>
  )
}
