import { request } from '@/utils/request';

export interface RuntimeInfo {
  go_version: string;
  os: string;
  arch: string;
  compiler: string;
}

export interface HealthStatus {
  status: string;
  time?: string;
  timestamp?: string;
  runtime?: RuntimeInfo;
  services?: Record<string, unknown>;
}

export function getHealth() {
  return request.get<HealthStatus>(
    {
      url: '/health',
    },
    {
      withToken: false,
    },
  );
}
