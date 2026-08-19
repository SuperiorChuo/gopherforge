import type { SmsProvider } from '@/api/system/sms'

export const providerLabels: Record<SmsProvider, string> = {
  debug: '调试',
  aliyun: '阿里云',
  tencent: '腾讯云',
}

export const providerColors: Record<SmsProvider, string> = {
  debug: 'default',
  aliyun: 'orange',
  tencent: 'blue',
}

export const templateTypeLabels: Record<number, string> = { 1: '验证码', 2: '通知', 3: '营销' }
export const templateTypeColors: Record<number, string> = { 1: 'blue', 2: 'green', 3: 'orange' }

// 与后端 pkg/sms 的占位规则一致：{name} 形式，字母数字下划线
const paramPattern = /\{([a-zA-Z0-9_]+)\}/g

export const extractParams = (content: string): string[] => {
  const keys: string[] = []
  for (const match of content.matchAll(paramPattern)) {
    if (!keys.includes(match[1])) keys.push(match[1])
  }
  return keys
}
