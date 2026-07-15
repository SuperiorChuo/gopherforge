import { useCountUp } from '@/hooks/useCountUp'

// 数字滚动展示：千分位格式化,配合 tabular-nums 不抖动
export default function CountUpValue({ value }: { value: number }) {
  const display = useCountUp(value)
  return <>{display.toLocaleString()}</>
}
