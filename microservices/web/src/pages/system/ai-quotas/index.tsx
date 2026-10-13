import React, { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Card,
  Table,
  Button,
  Tag,
  Modal,
  Form,
  Select,
  InputNumber,
  Switch,
  Alert,
} from 'antd';
import {
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  InfoCircleOutlined,
} from '@ant-design/icons';
import { message } from '@/utils/feedback';
import type { ColumnsType } from 'antd/es/table';
import GlassEmpty from '@/components/common/GlassEmpty';
import TableToolbar from '@/components/common/TableToolbar';
import TableRowActions from '@/components/common/TableRowActions';
import {
  getAIQuotaList,
  upsertAIQuota,
  deleteAIQuota,
  type AIQuota,
} from '@/api/ai/quota';

const AIQuotaManagement: React.FC = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [dataSource, setDataSource] = useState<AIQuota[]>([]);
  const [modalVisible, setModalVisible] = useState(false);
  const [editingQuota, setEditingQuota] = useState<AIQuota | null>(null);
  const [form] = Form.useForm();

  // 加载数据
  const loadData = async () => {
    setLoading(true);
    try {
      const list = await getAIQuotaList();
      setDataSource(list || []);
    } catch (error) {
      message.error(error instanceof Error ? error.message : t('加载失败'));
    } finally {
      setLoading(false);
    }
  };

  // 挂载时加载一次；后续由搜索/分页显式触发，loadData 身份刻意排除。
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => {
    loadData();
  }, []);

  // 打开新增/编辑模态框
  const handleOpenModal = (record?: AIQuota) => {
    setEditingQuota(record || null);
    if (record) {
      form.setFieldsValue(record);
    } else {
      form.resetFields();
      form.setFieldsValue({
        scope: 'global',
        reset_day_of_month: 1,
        enabled: 1,
      });
    }
    setModalVisible(true);
  };

  // 提交表单
  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();

      // 转换数值类型（处理空值）
      const data: Partial<AIQuota> = {
        ...values,
        enabled: values.enabled ? 1 : 0,
      };

      // scope_id 转换
      if (values.scope === 'global') {
        delete data.scope_id;
      } else {
        data.scope_id = Number(values.scope_id);
      }

      await upsertAIQuota(data);
      message.success(editingQuota ? t('更新成功') : t('创建成功'));

      setModalVisible(false);
      loadData();
    } catch (error) {
      if (error && typeof error === 'object' && 'errorFields' in error) {
        return; // 表单验证错误
      }
      message.error(error instanceof Error ? error.message : t('操作失败'));
    }
  };

  // 删除配额
  const handleDelete = async (id: number) => {
    try {
      await deleteAIQuota(id);
      message.success(t('删除成功'));
      loadData();
    } catch (error) {
      message.error(error instanceof Error ? error.message : t('删除失败'));
    }
  };

  const columns: ColumnsType<AIQuota> = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 60,
      responsive: ['lg'],
    },
    {
      title: t('作用域'),
      dataIndex: 'scope',
      width: 90,
      render: (scope, record) => {
        const colorMap: Record<string, string> = {
          global: 'blue',
          tenant: 'green',
          user: 'orange',
        };
        const labelMap: Record<string, string> = {
          global: '全局',
          tenant: '租户',
          user: '用户',
        };
        return (
          <div>
            <Tag color={colorMap[scope]}>{t(labelMap[scope] ?? '')}</Tag>
            {record.scope_id && (
              <span style={{ fontSize: 12, color: 'var(--text-tertiary)' }}>ID: {record.scope_id}</span>
            )}
          </div>
        );
      },
    },
    {
      title: t('每日限额'),
      width: 140,
      render: (_, record) => (
        <div style={{ fontSize: 12 }}>
          {record.daily_request_limit && <div>{t('请求 {{n}} 次', { n: record.daily_request_limit.toLocaleString() })}</div>}
          {record.daily_token_limit && <div>Token: {(record.daily_token_limit / 1000).toFixed(0)}K</div>}
          {record.daily_cost_limit && <div>{t('费用')}: ¥{record.daily_cost_limit.toFixed(2)}</div>}
          {!record.daily_request_limit && !record.daily_token_limit && !record.daily_cost_limit && (
            <span style={{ color: 'var(--text-tertiary)' }}>{t('不限')}</span>
          )}
        </div>
      ),
    },
    {
      title: t('每月限额'),
      width: 200,
      responsive: ['md'],
      render: (_, record) => (
        <div style={{ fontSize: 12 }}>
          {record.monthly_request_limit && <div>{t('请求 {{n}} 次', { n: record.monthly_request_limit.toLocaleString() })}</div>}
          {record.monthly_token_limit && <div>Token: {(record.monthly_token_limit / 1000).toFixed(0)}K</div>}
          {record.monthly_cost_limit && <div>{t('费用')}: ¥{record.monthly_cost_limit.toFixed(2)}</div>}
          {!record.monthly_request_limit && !record.monthly_token_limit && !record.monthly_cost_limit && (
            <span style={{ color: 'var(--text-tertiary)' }}>{t('不限')}</span>
          )}
        </div>
      ),
    },
    {
      title: t('重置日'),
      dataIndex: 'reset_day_of_month',
      width: 80,
      responsive: ['lg'],
      render: (day) => t('每月 {{n}} 日', { n: day }),
    },
    {
      title: t('状态'),
      dataIndex: 'enabled',
      width: 80,
      responsive: ['sm'],
      render: (enabled) => (
        <Tag color={enabled === 1 ? 'success' : 'default'}>
          {enabled === 1 ? t('启用') : t('禁用')}
        </Tag>
      ),
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
              confirm: t('确认删除该配额？'),
              onClick: () => { void handleDelete(record.id) },
            },
          ]}
        />
      ),
    },
  ];

  return (
    <div className="page-list ai-quota-page">
      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title="AI 配额管理"
          total={dataSource.length}
          extra={
            <Button type="primary" icon={<PlusOutlined />} onClick={() => handleOpenModal()}>
              {t('新增配额')}
            </Button>
          }
        />
        <Alert
          message={t('配额优先级')}
          description={t('用户级配额 > 租户级配额 > 全局配额。系统会按优先级检查配额，超限返回 429 错误。')}
          type="info"
          icon={<InfoCircleOutlined />}
          showIcon
          style={{ marginBottom: 16 }}
        />

        <Table
          loading={loading}
          dataSource={dataSource}
          columns={columns}
          rowKey="id"
          locale={{ emptyText: <GlassEmpty text={t('暂无配额，点右上角新增')} compact /> }}
          pagination={false}
         scroll={{ x: 960 }} />
      </Card>

      {/* 新增/编辑模态框 */}
      <Modal
        title={editingQuota ? t('编辑配额') : t('新增配额')}
        open={modalVisible}
        onOk={handleSubmit}
        onCancel={() => setModalVisible(false)}
        width={600}
        destroyOnClose
      >
        <Form form={form} layout="vertical" style={{ marginTop: 20 }}>
          <Form.Item
            label={t('作用域')}
            name="scope"
            rules={[{ required: true, message: t('请选择作用域') }]}
          >
            <Select>
              <Select.Option value="global">{t('全局（所有用户）')}</Select.Option>
              <Select.Option value="tenant">{t('租户')}</Select.Option>
              <Select.Option value="user">{t('用户')}</Select.Option>
            </Select>
          </Form.Item>

          <Form.Item noStyle shouldUpdate={(prev, curr) => prev.scope !== curr.scope}>
            {({ getFieldValue }) =>
              getFieldValue('scope') !== 'global' ? (
                <Form.Item
                  label={getFieldValue('scope') === 'tenant' ? t('租户 ID') : t('用户 ID')}
                  name="scope_id"
                  rules={[{ required: true, message: t('请输入 ID') }]}
                >
                  <InputNumber min={1} style={{ width: '100%' }} />
                </Form.Item>
              ) : null
            }
          </Form.Item>

          <Alert
            message={t('限额配置说明')}
            description={t('0 或空表示不限。每日限额在每日凌晨重置，每月限额在重置日重置。')}
            type="info"
            showIcon
            style={{ marginBottom: 16 }}
          />

          <Card title={t('每日限额')} size="small" style={{ marginBottom: 16 }}>
            <Form.Item label={t('请求次数上限')} name="daily_request_limit">
              <InputNumber min={0} placeholder="0" style={{ width: '100%' }} />
            </Form.Item>

            <Form.Item label={t('Token 用量上限')} name="daily_token_limit">
              <InputNumber min={0} placeholder="0" style={{ width: '100%' }} />
            </Form.Item>

            <Form.Item label={t('费用上限（元）')} name="daily_cost_limit">
              <InputNumber min={0} step={0.01} precision={2} placeholder="0" style={{ width: '100%' }} />
            </Form.Item>
          </Card>

          <Card title={t('每月限额')} size="small" style={{ marginBottom: 16 }}>
            <Form.Item label={t('请求次数上限')} name="monthly_request_limit">
              <InputNumber min={0} placeholder="0" style={{ width: '100%' }} />
            </Form.Item>

            <Form.Item label={t('Token 用量上限')} name="monthly_token_limit">
              <InputNumber min={0} placeholder="0" style={{ width: '100%' }} />
            </Form.Item>

            <Form.Item label={t('费用上限（元）')} name="monthly_cost_limit">
              <InputNumber min={0} step={0.01} precision={2} placeholder="0" style={{ width: '100%' }} />
            </Form.Item>

            <Form.Item
              label={t('重置日期')}
              name="reset_day_of_month"
              tooltip={t('每月几号重置配额（1-28）')}
            >
              <InputNumber min={1} max={28} style={{ width: '100%' }} />
            </Form.Item>
          </Card>

          <Form.Item label={t('状态')} name="enabled" valuePropName="checked">
            <Switch checkedChildren={t('启用')} unCheckedChildren={t('禁用')} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default AIQuotaManagement;
