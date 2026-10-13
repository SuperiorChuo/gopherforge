import React, { useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Card,
  Table,
  Button,
  Space,
  Tag,
  Modal,
  Form,
  Input,
  Select,
  InputNumber,
  Switch,
} from 'antd';
import { message } from '@/utils/feedback';
import {
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  EyeInvisibleOutlined,
  EyeOutlined,
} from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import GlassEmpty from '@/components/common/GlassEmpty';
import TableToolbar from '@/components/common/TableToolbar';
import TableRowActions from '@/components/common/TableRowActions';
import {
  getAIModelList,
  createAIModel,
  updateAIModel,
  deleteAIModel,
  type AIModel,
} from '@/api/ai/model';

const AIModelManagement: React.FC = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [dataSource, setDataSource] = useState<AIModel[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [modalVisible, setModalVisible] = useState(false);
  const [editingModel, setEditingModel] = useState<AIModel | null>(null);
  const [showApiKey, setShowApiKey] = useState<Record<number, boolean>>({});
  const [form] = Form.useForm();

  // 加载数据
  const loadData = useCallback(async () => {
    setLoading(true);
    try {
      const res = await getAIModelList({ page, page_size: pageSize });
      setDataSource(res.list || []);
      setTotal(res.total || 0);
    } catch (error) {
      message.error(error instanceof Error ? error.message : t('加载失败'));
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, t]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  // 打开新增/编辑模态框
  const handleOpenModal = (record?: AIModel) => {
    setEditingModel(record || null);
    if (record) {
      form.setFieldsValue({
        ...record,
        api_key: record.api_key === '********' ? '' : record.api_key,
      });
    } else {
      form.resetFields();
      form.setFieldsValue({
        status: 1,
        sort: 0,
        capabilities: [],
        cost_per_1k_input: 0,
        cost_per_1k_output: 0,
      });
    }
    setModalVisible(true);
  };

  // 提交表单
  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();

      // 如果 API Key 为空且是编辑，则不提交该字段
      if (editingModel && !values.api_key) {
        delete values.api_key;
      }

      if (editingModel) {
        await updateAIModel(editingModel.id, values);
        message.success(t('更新成功'));
      } else {
        await createAIModel(values);
        message.success(t('创建成功'));
      }

      setModalVisible(false);
      loadData();
    } catch (error) {
      if (error && typeof error === 'object' && 'errorFields' in error) {
        return; // 表单验证错误
      }
      message.error(error instanceof Error ? error.message : t('操作失败'));
    }
  };

  // 删除模型
  const handleDelete = async (id: number) => {
    try {
      await deleteAIModel(id);
      message.success(t('删除成功'));
      loadData();
    } catch (error) {
      message.error(error instanceof Error ? error.message : t('删除失败'));
    }
  };

  // 切换 API Key 显示
  const toggleApiKeyVisibility = (id: number) => {
    setShowApiKey(prev => ({ ...prev, [id]: !prev[id] }));
  };

  const columns: ColumnsType<AIModel> = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 60,
      responsive: ['lg'],
    },
    {
      title: t('模型名称'),
      dataIndex: 'display_name',
      width: 150,
      render: (text, record) => (
        <div>
          <div style={{ fontWeight: 500 }}>{text}</div>
          <div style={{ fontSize: 12, color: 'var(--text-tertiary)' }}>{record.name}</div>
        </div>
      ),
    },
    {
      title: t('供应商'),
      dataIndex: 'provider',
      width: 100,
      responsive: ['sm'],
      render: (provider) => {
        const colorMap: Record<string, string> = {
          openai: 'green',
          anthropic: 'blue',
          azure: 'orange',
          custom: 'purple',
        };
        return <Tag color={colorMap[provider] || 'default'}>{provider.toUpperCase()}</Tag>;
      },
    },
    {
      title: 'API Key',
      dataIndex: 'api_key',
      width: 150,
      responsive: ['md'],
      render: (key, record) => {
        if (!key || key === '********') {
          return <span style={{ color: 'var(--text-tertiary)' }}>{t('未配置')}</span>;
        }
        const visible = showApiKey[record.id];
        return (
          <Space>
            <span style={{ fontFamily: 'monospace' }}>
              {visible ? key : '••••••••••••••••'}
            </span>
            <Button
              type="text"
              size="small"
              icon={visible ? <EyeInvisibleOutlined /> : <EyeOutlined />}
              onClick={() => toggleApiKeyVisibility(record.id)}
            />
          </Space>
        );
      },
    },
    {
      title: t('能力'),
      dataIndex: 'capabilities',
      width: 120,
      responsive: ['md'],
      render: (capabilities: string[]) => (
        <Space size={4}>
          {capabilities?.map(cap => (
            <Tag key={cap} color="cyan" style={{ margin: 0 }}>
              {cap}
            </Tag>
          ))}
        </Space>
      ),
    },
    {
      title: t('费用（元/1K tokens）'),
      width: 180,
      responsive: ['lg'],
      render: (_, record) => (
        <div style={{ fontSize: 12 }}>
          <div>{t('输入')}: ¥{record.cost_per_1k_input.toFixed(4)}</div>
          <div>{t('输出')}: ¥{record.cost_per_1k_output.toFixed(4)}</div>
        </div>
      ),
    },
    {
      title: t('每日限额'),
      dataIndex: 'daily_limit',
      width: 100,
      responsive: ['lg'],
      render: (limit) => (limit ? t('{{n}} 次', { n: limit.toLocaleString() }) : t('不限')),
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 80,
      responsive: ['sm'],
      render: (status) => (
        <Tag color={status === 1 ? 'success' : 'default'}>
          {status === 1 ? t('启用') : t('禁用')}
        </Tag>
      ),
    },
    {
      title: t('排序'),
      dataIndex: 'sort',
      width: 70,
      responsive: ['lg'],
    },
    {
      title: t('操作'),
      key: 'action',
      fixed: 'right',
      width: 80,
      align: 'center' as const,
      render: (_, record) => (
        <TableRowActions
          maxInline={2}
          actions={[
            {
              key: 'edit',
              label: t('编辑'),
              icon: <EditOutlined />,
              onClick: () => handleOpenModal(record),
            },
            {
              key: 'delete',
              label: t('删除'),
              icon: <DeleteOutlined />,
              danger: true,
              confirm: t('确认删除该模型？'),
              onClick: () => { void handleDelete(record.id) },
            },
          ]}
        />
      ),
    },
  ];

  return (
    <div className="page-list ai-model-page">
      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title="AI 模型配置"
          total={total}
          extra={
            <Button type="primary" icon={<PlusOutlined />} onClick={() => handleOpenModal()}>
              {t('新增模型')}
            </Button>
          }
        />
        <Table
          loading={loading}
          dataSource={dataSource}
          columns={columns}
          rowKey="id"
          locale={{ emptyText: <GlassEmpty text={t('暂无模型，点右上角新增')} compact /> }}
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (n) => t('共 {{n}} 条', { n }),
            onChange: (page, pageSize) => {
              setPage(page);
              setPageSize(pageSize);
            },
          }}
         scroll={{ x: 960 }} />
      </Card>

      {/* 新增/编辑模态框 */}
      <Modal
        title={editingModel ? t('编辑模型') : t('新增模型')}
        open={modalVisible}
        onOk={handleSubmit}
        onCancel={() => setModalVisible(false)}
        width={700}
        destroyOnClose
      >
        <Form form={form} layout="vertical" style={{ marginTop: 20 }}>
          <Form.Item
            label={t('模型标识')}
            name="name"
            rules={[{ required: true, message: t('请输入模型标识') }]}
            tooltip={t('如：gpt-4o、claude-3-5-sonnet-20241022')}
          >
            <Input placeholder="gpt-4o" />
          </Form.Item>

          <Form.Item
            label={t('显示名称')}
            name="display_name"
            rules={[{ required: true, message: t('请输入显示名称') }]}
          >
            <Input placeholder="GPT-4o" />
          </Form.Item>

          <Form.Item
            label={t('供应商')}
            name="provider"
            rules={[{ required: true, message: t('请选择供应商') }]}
          >
            <Select>
              <Select.Option value="openai">OpenAI</Select.Option>
              <Select.Option value="anthropic">Anthropic</Select.Option>
              <Select.Option value="azure">Azure</Select.Option>
              <Select.Option value="custom">{t('自定义')}</Select.Option>
            </Select>
          </Form.Item>

          <Form.Item
            label="API Key"
            name="api_key"
            rules={[
              { required: !editingModel, message: t('请输入 API Key') },
            ]}
            tooltip={editingModel ? t('留空则不修改现有密钥') : undefined}
          >
            <Input.Password
              placeholder={editingModel ? t('留空不修改') : 'sk-proj-xxx'}
              autoComplete="new-password"
            />
          </Form.Item>

          <Form.Item
            label="Base URL"
            name="base_url"
            tooltip={t('自定义端点（如 Azure 或私有部署），留空使用默认')}
          >
            <Input placeholder="https://api.openai.com" />
          </Form.Item>

          <Form.Item
            label={t('能力标识')}
            name="capabilities"
            rules={[{ required: true, message: t('请选择能力标识') }]}
          >
            <Select mode="multiple">
              <Select.Option value="chat">{t('Chat（对话）')}</Select.Option>
              <Select.Option value="embed">{t('Embed（向量）')}</Select.Option>
              <Select.Option value="image">{t('Image（图片生成）')}</Select.Option>
            </Select>
          </Form.Item>

          <Form.Item label={t('费用配置（元/1K tokens）')} style={{ marginBottom: 0 }}>
            <Space>
              <Form.Item
                name="cost_per_1k_input"
                rules={[{ required: true, message: t('请输入') }]}
              >
                <InputNumber
                  min={0}
                  step={0.001}
                  precision={6}
                  placeholder={t('输入 token')}
                  style={{ width: 150 }}
                />
              </Form.Item>
              <Form.Item
                name="cost_per_1k_output"
                rules={[{ required: true, message: t('请输入') }]}
              >
                <InputNumber
                  min={0}
                  step={0.001}
                  precision={6}
                  placeholder={t('输出 token')}
                  style={{ width: 150 }}
                />
              </Form.Item>
            </Space>
          </Form.Item>

          <Form.Item label={t('每日调用上限')} name="daily_limit" tooltip={t('0 或空表示不限')}>
            <InputNumber min={0} placeholder="0" style={{ width: '100%' }} />
          </Form.Item>

          <Form.Item label={t('描述')} name="description">
            <Input.TextArea rows={3} placeholder={t('模型描述')} />
          </Form.Item>

          <Form.Item label={t('排序权重')} name="sort" tooltip={t('数字越小越靠前')}>
            <InputNumber min={0} style={{ width: '100%' }} />
          </Form.Item>

          <Form.Item label={t('状态')} name="status" valuePropName="checked">
            <Switch checkedChildren={t('启用')} unCheckedChildren={t('禁用')} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default AIModelManagement;
