import request from '@/utils/request'

export interface LiveWeather {
  city: string
  adcode: string
  weather: string
  temperature: string
  humidity: string
  wind_dir: string
  wind_power: string
  report_time: string
  /** 今日最高温（预报，可选） */
  temp_high?: string
  /** 今日最低温（预报，可选） */
  temp_low?: string
}

// 天气是装饰性信息：silent 抑制全局错误提示，失败由调用方静默处理
export const getLiveWeather = () =>
  request.get<unknown, LiveWeather>('/api/v1/system/weather', { silent: true })
