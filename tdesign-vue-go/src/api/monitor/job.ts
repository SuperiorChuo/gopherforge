import { typedApi } from '@/api/generated/client';
import type { components } from '@/api/generated/schema';

type Schema<Name extends keyof components['schemas']> = components['schemas'][Name];

export type ScheduledJob = Schema<'ScheduledJob'>;
export type JobListResponse = Schema<'JobListResponse'>;
export type JobAbnormalStatus = Schema<'JobAbnormalStatus'>;
export type JobHealthCheck = Schema<'JobHealthCheck'>;
export type JobLogCleanupRequest = Schema<'JobLogCleanupRequest'>;
export type JobLogCleanupResult = Schema<'JobLogCleanupResult'>;
export type SaveJobRequest = Schema<'SaveJobRequest'>;

export interface JobListRequest {
  page?: number;
  page_size?: number;
  name?: string;
  status?: number;
}

export function getJobList(params?: JobListRequest) {
  return typedApi.get('/api/v1/monitor/jobs', {
    query: params,
  });
}

export function createJob(data: Partial<SaveJobRequest>) {
  return typedApi.post('/api/v1/monitor/jobs', {
    body: data as SaveJobRequest,
  });
}

export function updateJob(id: number, data: Partial<SaveJobRequest>) {
  return typedApi.put('/api/v1/monitor/jobs/{id}', {
    path: { id },
    body: data as SaveJobRequest,
  });
}

export function deleteJob(id: number) {
  return typedApi.delete('/api/v1/monitor/jobs/{id}', {
    path: { id },
  });
}

export function startJob(id: number) {
  return typedApi.post('/api/v1/monitor/jobs/{id}/start', {
    path: { id },
  });
}

export function stopJob(id: number) {
  return typedApi.post('/api/v1/monitor/jobs/{id}/stop', {
    path: { id },
  });
}

export function runJob(id: number) {
  return typedApi.post('/api/v1/monitor/jobs/{id}/run', {
    path: { id },
  });
}

export function getJobHealth(params?: { window_hours?: number }) {
  return typedApi.get('/api/v1/monitor/jobs/health', {
    query: params,
  });
}

export function cleanupJobLogs(data: JobLogCleanupRequest) {
  return typedApi.post('/api/v1/monitor/job-logs/cleanup', {
    body: data,
  });
}
