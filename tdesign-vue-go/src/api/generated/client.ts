import { request } from '@/utils/request';
import type { RequestOptions } from '@/types/axios';

import type { paths } from './schema';

type HttpMethod = 'get' | 'post' | 'put' | 'delete' | 'patch';

type PathByMethod<M extends HttpMethod> = {
  [P in keyof paths]: M extends keyof paths[P] ? P : never;
}[keyof paths] &
  string;

type OperationFor<P extends keyof paths, M extends HttpMethod> = M extends keyof paths[P] ? paths[P][M] : never;

type PathParams<Op> = Op extends { parameters: { path: infer Params } } ? Params : never;
type QueryParams<Op> = Op extends { parameters: { query: infer Params } } ? Params : never;
type JsonBody<Op> = Op extends { requestBody: { content: { 'application/json': infer Body } } } ? Body : never;

type JsonResponse<Op> = Op extends { responses: { '200': { content: { 'application/json': infer Response } } } }
  ? Response
  : unknown;

type UnwrapResponseData<Response> = Response extends { data: infer Data }
  ? Data extends Record<string, never>
    ? void
    : Data
  : Response;

export type ResponseData<Op> = UnwrapResponseData<JsonResponse<Op>>;

type PathInput<Op> = [PathParams<Op>] extends [never] ? { path?: never } : { path: PathParams<Op> };
type RequiredKeys<T> = keyof {
  [K in keyof T as Record<string, never> extends Pick<T, K> ? never : K]: T[K];
};
type HasRequiredKeys<T> = RequiredKeys<T> extends never ? false : true;
type QueryInput<Op> = [QueryParams<Op>] extends [never]
  ? { query?: never }
  : HasRequiredKeys<QueryParams<Op>> extends true
    ? { query: QueryParams<Op> }
    : { query?: QueryParams<Op> };
type BodyInput<Op> = [JsonBody<Op>] extends [never] ? { body?: never } : { body: JsonBody<Op> };

export type TypedRequestOptions<Op> = PathInput<Op> & QueryInput<Op> & BodyInput<Op> & RequestOptions;

type HasPathInput<Op> = [PathParams<Op>] extends [never] ? false : true;
type HasBodyInput<Op> = [JsonBody<Op>] extends [never] ? false : true;
type HasRequiredQueryInput<Op> = [QueryParams<Op>] extends [never] ? false : HasRequiredKeys<QueryParams<Op>>;
type NeedsRequestOptions<Op> = HasPathInput<Op> extends true
  ? true
  : HasBodyInput<Op> extends true
    ? true
    : HasRequiredQueryInput<Op>;
type TypedRequestArgs<Op> = NeedsRequestOptions<Op> extends true
  ? [options: TypedRequestOptions<Op>]
  : [options?: TypedRequestOptions<Op>];

type RuntimeRequestOptions = RequestOptions & {
  path?: Record<string, string | number>;
  query?: Record<string, unknown>;
  body?: unknown;
};

export function buildApiPath(template: string, params?: Record<string, string | number>) {
  return template.replace(/\{([^}]+)\}/g, (_, name: string) => {
    const value = params?.[name];
    if (value === undefined || value === null) {
      throw new Error(`Missing path parameter: ${name}`);
    }
    return encodeURIComponent(String(value));
  });
}

function toRequestUrl(path: string) {
  return path.replace(/^\/api\/v1/, '') || '/';
}

function typedRequest<M extends HttpMethod, P extends PathByMethod<M>>(
  method: M,
  path: P,
  ...args: TypedRequestArgs<OperationFor<P, M>>
): Promise<ResponseData<OperationFor<P, M>>> {
  const [options] = args;
  const { path: pathParams, query, body, ...requestOptions } = (options || {}) as RuntimeRequestOptions;
  const url = toRequestUrl(buildApiPath(path, pathParams));
  const config = {
    url,
    params: query,
    data: body,
  };

  switch (method) {
    case 'get':
      return request.get(config, requestOptions);
    case 'post':
      return request.post(config, requestOptions);
    case 'put':
      return request.put(config, requestOptions);
    case 'delete':
      return request.delete(config, requestOptions);
    case 'patch':
      return request.patch(config, requestOptions);
    default:
      throw new Error(`Unsupported method: ${method}`);
  }
}

export const typedApi = {
  get<P extends PathByMethod<'get'>>(path: P, ...args: TypedRequestArgs<OperationFor<P, 'get'>>) {
    return typedRequest('get', path, ...args);
  },
  post<P extends PathByMethod<'post'>>(path: P, ...args: TypedRequestArgs<OperationFor<P, 'post'>>) {
    return typedRequest('post', path, ...args);
  },
  put<P extends PathByMethod<'put'>>(path: P, ...args: TypedRequestArgs<OperationFor<P, 'put'>>) {
    return typedRequest('put', path, ...args);
  },
  delete<P extends PathByMethod<'delete'>>(path: P, ...args: TypedRequestArgs<OperationFor<P, 'delete'>>) {
    return typedRequest('delete', path, ...args);
  },
  patch<P extends PathByMethod<'patch'>>(path: P, ...args: TypedRequestArgs<OperationFor<P, 'patch'>>) {
    return typedRequest('patch', path, ...args);
  },
};
