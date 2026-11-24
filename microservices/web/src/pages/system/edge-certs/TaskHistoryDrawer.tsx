import { Drawer, List, Space, Tag, Typography } from 'antd'
import GlassEmpty from '@/components/common/GlassEmpty'
import { formatDateTime } from '@/utils/format'
import { CertStatusTag } from './CertStatusTags'
import { taskKindMeta, taskStatusMeta, type TaskModalState } from './helpers'

export interface TaskHistoryDrawerProps {
  taskModal: TaskModalState
  compact: boolean
  onClose: () => void
}

export function TaskHistoryDrawer({ taskModal, compact, onClose }: TaskHistoryDrawerProps) {
  return (
    <Drawer
      title={taskModal ? `${taskModal.row.domain} · 任务记录` : '任务记录'}
      open={!!taskModal}
      onClose={onClose}
      width={compact ? '100%' : 560}
    >
      <List
        loading={taskModal?.loading}
        dataSource={taskModal?.tasks ?? []}
        locale={{ emptyText: <GlassEmpty text="暂无任务记录" /> }}
        renderItem={(task) => (
          <List.Item>
            <List.Item.Meta
              title={(
                <Space wrap>
                  <Typography.Text strong>{taskKindMeta[task.kind]?.label || task.kind}</Typography.Text>
                  <CertStatusTag value={task.status} meta={taskStatusMeta} />
                  {task.environment === 'staging' && <Tag color="warning">Staging</Tag>}
                </Space>
              )}
              description={(
                <Space direction="vertical" size={2}>
                  <Typography.Text type="secondary">
                    提交：{formatDateTime(task.created_at)}
                    {task.finished_at ? ` · 完成：${formatDateTime(task.finished_at)}` : ''}
                  </Typography.Text>
                  {task.step && <Typography.Text type="secondary">阶段：{task.step}</Typography.Text>}
                  {task.error_message && <Typography.Text type="danger">{task.error_message}</Typography.Text>}
                  {task.error_hint && <Typography.Text type="secondary">建议：{task.error_hint}</Typography.Text>}
                </Space>
              )}
            />
          </List.Item>
        )}
      />
    </Drawer>
  )
}
