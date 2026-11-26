import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, Button, Form, Input, Modal, Select, Space, Table, Tag, Typography } from 'antd'
import { BulbOutlined } from '@ant-design/icons'
import { message } from '@/utils/feedback'
import {
  batchCreateGeoPrompts,
  createBrand,
  generateGeoPrompts,
  type GeneratedPrompt,
  type SuggestedBrand,
} from '@/api/visibility'

const { Text, Paragraph } = Typography

type GeneratePromptsFormValues = {
  site_url?: string
  hint?: string
  count?: number
}

export interface GeneratePromptsModalProps {
  open: boolean
  onClose: () => void
  onSaved: () => Promise<void> | void
}

export default function GeneratePromptsModal({
  open,
  onClose,
  onSaved,
}: GeneratePromptsModalProps) {
  const { t } = useTranslation()
  const [genForm] = Form.useForm<GeneratePromptsFormValues>()
  const [generating, setGenerating] = useState(false)
  const [savingGen, setSavingGen] = useState(false)
  const [candidates, setCandidates] = useState<GeneratedPrompt[]>([])
  const [selectedGen, setSelectedGen] = useState<number[]>([])
  const [suggestedBrand, setSuggestedBrand] = useState<SuggestedBrand | null>(null)
  const [addingBrand, setAddingBrand] = useState(false)
  const [brandAdded, setBrandAdded] = useState(false)

  const resetCandidates = useCallback(() => {
    setCandidates([])
    setSelectedGen([])
    setSuggestedBrand(null)
    setBrandAdded(false)
  }, [])

  useEffect(() => {
    if (!open) return
    genForm.resetFields()
  }, [open, genForm])

  async function onGenerate() {
    const values = await genForm.validateFields().catch(() => null)
    if (!values) return
    setGenerating(true)
    try {
      const res = await generateGeoPrompts({
        site_url: values.site_url?.trim() || undefined,
        hint: values.hint?.trim() || undefined,
        count: values.count,
      })
      const items = res.items ?? []
      setCandidates(items)
      setSelectedGen(items.map((_, i) => i))
      setSuggestedBrand(res.suggested_brand ?? null)
      setBrandAdded(false)
      if (res.site_fetched === false) {
        message.warning(t('网页内容抓取失败（服务器访问不了该站点），已根据网址和 AI 已有认知生成，请核对后再入库'))
      }
      if (items.length === 0) message.info(t('AI 这次没给出问题，换个描述再试试'))
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('AI 生成失败'))
    } finally {
      setGenerating(false)
    }
  }

  async function onAddSuggestedBrand() {
    if (!suggestedBrand) return
    setAddingBrand(true)
    try {
      await createBrand({ name: suggestedBrand.name, aliases: suggestedBrand.aliases, kind: 'brand' })
      setBrandAdded(true)
      message.success(t('品牌「{{name}}」已加入品牌库', { name: suggestedBrand.name }))
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('加入品牌库失败'))
    } finally {
      setAddingBrand(false)
    }
  }

  function editCandidate(idx: number, patch: Partial<GeneratedPrompt>) {
    setCandidates((prev) => prev.map((it, i) => (i === idx ? { ...it, ...patch } : it)))
  }

  async function onSaveGen() {
    const picks = selectedGen
      .map((i) => candidates[i])
      .filter((it): it is GeneratedPrompt => Boolean(it && it.question.trim()))
    if (picks.length === 0) {
      message.warning(t('先勾选要加入的问题'))
      return
    }
    setSavingGen(true)
    try {
      const res = await batchCreateGeoPrompts(picks)
      message.success(res.skipped > 0
        ? t('已加入 {{a}} 个问题，跳过 {{b}} 个重复', { a: res.created, b: res.skipped })
        : t('已加入 {{n}} 个问题', { n: res.created }))
      onClose()
      await onSaved()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('加入失败'))
    } finally {
      setSavingGen(false)
    }
  }

  return (
    <Modal
      title={t('AI 生成检测问题')}
      open={open}
      onCancel={onClose}
      afterClose={resetCandidates}
      width={720}
      destroyOnHidden
      footer={[
        <Button key="cancel" onClick={onClose}>{t('取消')}</Button>,
        <Button key="gen" icon={<BulbOutlined />} loading={generating} onClick={() => void onGenerate()}>
          {candidates.length > 0 ? t('重新生成') : t('开始生成')}
        </Button>,
        <Button key="save" type="primary" loading={savingGen} disabled={selectedGen.length === 0} onClick={() => void onSaveGen()}>
          {t('加入选中（{{n}}）', { n: selectedGen.length })}
        </Button>,
      ]}
    >
      <Form form={genForm} layout="vertical" initialValues={{ count: 8 }}>
        <Form.Item
          name="site_url"
          label={t('网址（可选）')}
          rules={[{ pattern: /^https?:\/\/.+/i, message: t('以 http:// 或 https:// 开头') }]}
          help={t('填了就抓取这个网页的内容来生成问题，并自动识别站点品牌；不填则按「品牌与竞品」里的资料生成')}
        >
          <Input allowClear placeholder={t('例：https://your-product.com')} />
        </Form.Item>
        <Form.Item
          name="hint"
          label={t('行业/受众补充说明（可选）')}
          help={t('填了能更贴合，例如目标客户、使用场景、地域')}
        >
          <Input.TextArea rows={2} placeholder={t('例：面向国内中小企业技术团队的开源后台框架')} />
        </Form.Item>
        <Form.Item name="count" label={t('生成数量')}>
          <Select
            options={[5, 8, 12, 20].map((n) => ({ label: t('{{n}} 个', { n }), value: n }))}
            style={{ width: 140 }}
          />
        </Form.Item>
      </Form>
      {suggestedBrand ? (
        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 12 }}
          message={
            <Space wrap>
              <span>
                {t('识别到站点品牌：')}<Text strong>{suggestedBrand.name}</Text>
                {suggestedBrand.aliases ? <Text type="secondary">{t('（别名：{{name}}）', { name: suggestedBrand.aliases })}</Text> : null}
              </span>
              {brandAdded ? (
                <Tag color="green" variant="filled">{t('已加入品牌库')}</Tag>
              ) : (
                <Button size="small" type="primary" ghost loading={addingBrand} onClick={() => void onAddSuggestedBrand()}>
                  {t('加入品牌库')}
                </Button>
              )}
            </Space>
          }
        />
      ) : null}
      {candidates.length > 0 ? (
        <>
          <Text type="secondary">{t('勾选要加入的问题，问题文字和分类都可以直接修改；入库时自动跳过与题库重复的：')}</Text>
          <Table
            style={{ marginTop: 8 }}
            rowKey={(_, i) => String(i)}
            size="small"
            pagination={false}
            scroll={{ y: 320 }}
            rowSelection={{
              selectedRowKeys: selectedGen.map(String),
              onChange: (keys) => setSelectedGen(keys.map((k) => Number(k))),
            }}
            columns={[
              {
                title: t('分类'),
                dataIndex: 'category',
                width: 120,
                render: (v, _r, idx) => (
                  <Select
                    size="small"
                    value={v as string}
                    style={{ width: 104 }}
                    onChange={(nv) => editCandidate(idx, { category: nv })}
                    options={[
                      { label: t('品类词'), value: 'category' },
                      { label: t('对比题'), value: 'comparison' },
                      { label: t('场景题'), value: 'scenario' },
                      { label: t('其他'), value: 'other' },
                    ]}
                  />
                ),
              },
              {
                title: t('问题（可编辑）'),
                dataIndex: 'question',
                render: (v, _r, idx) => (
                  <Input.TextArea
                    size="small"
                    autoSize={{ minRows: 1, maxRows: 3 }}
                    value={v as string}
                    onChange={(e) => editCandidate(idx, { question: e.target.value })}
                  />
                ),
              },
            ]}
            dataSource={candidates}
          />
        </>
      ) : (
        <Paragraph type="secondary" style={{ marginBottom: 0 }}>
          {t('填一个产品官网网址（或留空用品牌资料），点「开始生成」，AI 会站在潜在用户角度想出他们可能向 AI 提问的问题。生成后可编辑、勾选加入检测题库。')}
        </Paragraph>
      )}
    </Modal>
  )
}
