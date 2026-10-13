import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { listNotifyTemplates, type NotifyTemplate } from '@/api/notify'
import SimpleListPage from '../SimpleListPage'

const CHANNEL_LABEL: Record<string, string> = {
  inbox: '站内信',
  email: '邮件',
  sms: '短信',
}

function channelText(row: NotifyTemplate) {
  const raw = row.channels || row.channel || ''
  return raw
    .split(',')
    .map((part) => part.trim())
    .filter(Boolean)
    .map((part) => CHANNEL_LABEL[part] ?? part)
    .join(' · ')
}

export default function MobileTemplatesPage() {
  const { t } = useTranslation()
  const [open, setOpen] = useState<NotifyTemplate | null>(null)

  const load = useCallback(async () => {
    const res = await listNotifyTemplates()
    const list = res.list ?? []
    return { list, total: list.length }
  }, [])

  return (
    <>
      <SimpleListPage
        title="通知模板"
        emptyText="暂无通知模板"
        load={load}
        rowKey={(row: NotifyTemplate) => row.id}
        render={(row) => (
          <button type="button" className="m-row" onClick={() => setOpen(row)}>
            <div className="m-row-top">
              <span className={`m-pill${row.enabled ? '' : ' is-bad'}`}>
                {row.enabled ? t('启用') : t('停用')}
              </span>
              <span>{channelText(row) || t('通知模板')}</span>
            </div>
            <strong>{row.code}</strong>
            <p>{row.title_tpl || t('查看模板')}</p>
          </button>
        )}
      />
      {open && (
        <div className="m-sheet" role="dialog" aria-modal="true" aria-labelledby="m-tpl-title">
          <div className="m-sheet-card">
            <div className="m-row-top">
              <span className={`m-pill${open.enabled ? '' : ' is-bad'}`}>
                {open.enabled ? t('启用') : t('停用')}
              </span>
              <span>{channelText(open) || '--'}</span>
            </div>
            <h3 id="m-tpl-title">{open.code}</h3>
            <dl className="m-kv">
              <div>
                <dt>{t('标题')}</dt>
                <dd>{open.title_tpl || '--'}</dd>
              </div>
              <div>
                <dt>{t('正文')}</dt>
                <dd>{open.body_tpl || t('暂无内容')}</dd>
              </div>
              {open.link_tpl ? (
                <div>
                  <dt>{t('链接')}</dt>
                  <dd>{open.link_tpl}</dd>
                </div>
              ) : null}
            </dl>
            <p className="m-meta">{t('编辑模板请回电脑')}</p>
            <div className="m-actions">
              <button type="button" className="m-text-btn" onClick={() => setOpen(null)}>
                {t('关闭')}
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  )
}
