// 与仓库根目录 CONTRIBUTING.md / AGENTS.md 的全中文提交规范对齐。
// 标题格式：类型（可选范围）：说明
// 例：功能：新增某某能力
// 例：修复（React Ant Design 前端）：修复某某问题
export default {
  parserPreset: {
    parserOpts: {
      headerPattern:
        /^(功能|修复|样式|重构|文档|测试|杂项|性能|持续集成|合并|构建|回滚)(?:（([^）]+)）)?：(.+)$/,
      headerCorrespondence: ['type', 'scope', 'subject'],
    },
  },
  rules: {
    'type-empty': [2, 'never'],
    'subject-empty': [2, 'never'],
    'type-enum': [
      2,
      'always',
      [
        '功能',
        '修复',
        '样式',
        '重构',
        '文档',
        '测试',
        '杂项',
        '性能',
        '持续集成',
        '合并',
        '构建',
        '回滚',
      ],
    ],
    // 中文标题不做英文大小写约束
    'subject-case': [0],
    'subject-full-stop': [0],
    'header-max-length': [2, 'always', 100],
  },
};
