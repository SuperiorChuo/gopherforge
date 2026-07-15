import { Fragment, type JSX, type ReactNode } from 'react'

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
  type: 'code' | 'heading' | 'ul' | 'ol' | 'quote' | 'hr' | 'p'
  lines: string[]
  level?: number
  lang?: string
}

function parseBlocks(md: string): Block[] {
  const lines = md.replace(/\r\n/g, '\n').split('\n')
  const blocks: Block[] = []
  let idx = 0
  while (idx < lines.length) {
    const line = lines[idx]
    // 代码块
    const fence = line.match(/^```(\S*)\s*$/)
    if (fence) {
      const code: string[] = []
      idx += 1
      while (idx < lines.length && !/^```\s*$/.test(lines[idx])) {
        code.push(lines[idx])
        idx += 1
      }
      idx += 1 // 跳过闭合 ```（流式渲染时可能还没到，也照常收尾）
      blocks.push({ type: 'code', lines: code, lang: fence[1] || undefined })
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
      !/^\s*(-{3,}|\*{3,})\s*$/.test(lines[idx])
    ) {
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
            const Tag = `h${level}` as keyof JSX.IntrinsicElements
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
