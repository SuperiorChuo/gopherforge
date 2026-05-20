// axios配置  可自行根据项目进行更改，只需更改该文件即可，其他文件可以不动
import type { AxiosInstance } from 'axios';
import isString from 'lodash/isString';
import merge from 'lodash/merge';

import { ContentTypeEnum } from '@/constants';
import { useUserStore } from '@/store';

import { VAxios } from './Axios';
import type { AxiosTransform, CreateAxiosOptions } from './AxiosTransform';
import { formatRequestDate, joinTimestamp, setObjToUrlParams } from './utils';

const env = import.meta.env.MODE || 'development';

// 如果是mock模式 或 没启用直连代理 就不配置host 会走本地Mock拦截 或 Vite 代理
const host = env === 'mock' || import.meta.env.VITE_IS_REQUEST_PROXY !== 'true' ? '' : import.meta.env.VITE_API_URL;
const apiPrefix = import.meta.env.VITE_API_URL_PREFIX || '/api/v1';
const refreshUrl = `${host}${apiPrefix}/refresh`;

function getResponseMessage(response?: any) {
  return response?.data?.message || response?.data?.msg || response?.data?.detail || '';
}

function isRefreshRequest(config?: any) {
  const url = config?.url || '';
  return url === refreshUrl || url.endsWith(`${apiPrefix}/refresh`) || url.endsWith('/refresh');
}

function redirectToLogin(userStore: ReturnType<typeof useUserStore>) {
  userStore.logout();
  if (window.location.pathname !== '/login') {
    window.location.href = '/login';
  }
}

// 数据处理，方便区分多种处理方式
const transform: AxiosTransform = {
  // 处理请求数据。如果数据不是预期格式，可直接抛出错误
  transformRequestHook: (res, options) => {
    const { isTransformResponse, isReturnNativeResponse } = options;

    // 如果204无内容直接返回
    const method = res.config.method?.toLowerCase();
    if (res.status === 204 && ['put', 'patch', 'delete'].includes(method || '')) {
      return res;
    }

    // 是否返回原生响应头 比如：需要获取响应头时使用该属性
    if (isReturnNativeResponse) {
      return res;
    }
    // 不进行任何处理，直接返回
    // 用于页面代码可能需要直接获取code，data，message这些信息时开启
    if (!isTransformResponse) {
      return res.data;
    }

    // 错误的时候返回
    const { data } = res;
    if (!data) {
      throw new Error('请求接口错误');
    }

    //  这里 code为 后台统一的字段，需要在 types.ts内修改为项目自己的接口返回格式
    const { code } = data;

    // 这里逻辑可以根据项目进行修改
    const hasSuccess = data && (code === 200 || code === 0);
    if (hasSuccess) {
      return data.data;
    }

    // 处理 401 未授权错误（登录失败）
    if (code === 401) {
      const userStore = useUserStore();
      // 只有在非登录页面时才清除 token 和跳转
      if (window.location.pathname !== '/login') {
        userStore.logout();
        window.location.href = '/login';
      }
    }

    throw new Error(data.message || `请求接口错误, 错误码: ${code}`);
  },

  // 请求前处理配置
  beforeRequestHook: (config, options) => {
    const { apiUrl, isJoinPrefix, urlPrefix, joinParamsToUrl, formatDate, joinTime = true } = options;

    // 添加接口前缀
    if (isJoinPrefix && urlPrefix && isString(urlPrefix)) {
      config.url = `${urlPrefix}${config.url}`;
    }

    // 将baseUrl拼接
    if (apiUrl && isString(apiUrl)) {
      config.url = `${apiUrl}${config.url}`;
    }
    const params = config.params || {};
    const data = config.data || false;

    if (formatDate && data && !isString(data)) {
      formatRequestDate(data);
    }
    if (config.method?.toUpperCase() === 'GET') {
      if (!isString(params)) {
        // 给 get 请求加上时间戳参数，避免从缓存中拿数据。
        config.params = Object.assign(params || {}, joinTimestamp(joinTime, false));
      } else {
        // 兼容restful风格
        config.url = `${config.url + params}${joinTimestamp(joinTime, true)}`;
        config.params = undefined;
      }
    } else if (!isString(params)) {
      if (formatDate) {
        formatRequestDate(params);
      }
      if (
        Reflect.has(config, 'data') &&
        config.data &&
        (Object.keys(config.data).length > 0 || data instanceof FormData)
      ) {
        config.data = data;
        config.params = params;
      } else {
        // 非GET请求如果没有提供data，则将params视为data
        config.data = params;
        config.params = undefined;
      }
      if (joinParamsToUrl) {
        config.url = setObjToUrlParams(config.url as string, { ...config.params, ...config.data });
      }
    } else {
      // 兼容restful风格
      config.url += params;
      config.params = undefined;
    }
    return config;
  },

  // 请求拦截器处理
  requestInterceptors: (config, options) => {
    // 请求之前处理config
    const userStore = useUserStore();
    const { token } = userStore;

    if (token && (config as Recordable)?.requestOptions?.withToken !== false) {
      // jwt token
      (config as Recordable).headers.Authorization = options.authenticationScheme
        ? `${options.authenticationScheme} ${token}`
        : `Bearer ${token}`;
    }
    return config;
  },

  // 响应拦截器处理
  responseInterceptors: (res) => {
    return res;
  },

  // 响应错误处理
  responseInterceptorsCatch: async (error: any, instance: AxiosInstance) => {
    const { response, config } = error;
    const userStore = useUserStore();
    const responseMessage = getResponseMessage(response);

    // 处理 HTTP 状态码错误（如 404, 500 等）
    if (response?.status && (response.status < 200 || response.status >= 300)) {
      // HTTP 401 未授权 - Token过期
      if (response.status === 401) {
        if (isRefreshRequest(config)) {
          redirectToLogin(userStore);
          window.isRefreshingToken = false;
          return Promise.reject(new Error(responseMessage || '登录已过期，请重新登录'));
        }

        // 检查是否已经在刷新Token
        if (!window.isRefreshingToken) {
          window.isRefreshingToken = true;

          try {
            // 使用refreshToken获取新的accessToken
            const refreshToken = userStore.refreshToken;
            if (!refreshToken) {
              throw new Error('No refresh token available');
            }

            // 直接调用refreshToken API，避免循环刷新
            const refreshRes = await instance.post(
              refreshUrl,
              {
                refresh_token: refreshToken,
              },
              {
                headers: {
                  Authorization: '',
                },
              },
            );

            if (refreshRes.data.code === 200 && refreshRes.data.data?.access_token) {
              // 更新Token信息
              const newAccessToken = refreshRes.data.data.access_token;
              userStore.token = newAccessToken;
              if (refreshRes.data.data.refresh_token) {
                userStore.refreshToken = refreshRes.data.data.refresh_token;
              }

              // 更新当前请求的Authorization头
              config.headers = config.headers || {};
              config.headers.Authorization = `Bearer ${newAccessToken}`;

              // 重置刷新状态
              window.isRefreshingToken = false;

              // 重新发送原始请求
              return instance(config);
            } else {
              throw new Error('Failed to refresh token');
            }
          } catch {
            // 刷新Token失败，清除用户信息并跳转到登录页
            userStore.logout();
            if (window.location.pathname !== '/login') {
              window.location.href = '/login';
            }
            window.isRefreshingToken = false;
            return Promise.reject(new Error('登录已过期，请重新登录'));
          }
        } else {
          // 等待Token刷新完成后重试
          return new Promise((resolve, reject) => {
            const checkRefresh = () => {
              if (!window.isRefreshingToken) {
                if (!userStore.token) {
                  reject(new Error('登录已过期，请重新登录'));
                  return;
                }
                config.headers = config.headers || {};
                config.headers.Authorization = `Bearer ${userStore.token}`;
                resolve(instance(config));
              } else {
                setTimeout(checkRefresh, 100);
              }
            };
            checkRefresh();
          });
        }
      }
      return Promise.reject(new Error(responseMessage || `请求失败: ${response.status}`));
    }

    // 处理网络错误或其他错误
    if (!response) {
      return Promise.reject(new Error('网络错误，请检查网络连接'));
    }

    // 重试逻辑
    if (!config || !config.requestOptions.retry) return Promise.reject(error);

    config.retryCount = config.retryCount || 0;

    if (config.retryCount >= config.requestOptions.retry.count) return Promise.reject(error);

    config.retryCount += 1;

    const backoff = new Promise<typeof config>((resolve) => {
      setTimeout(() => {
        resolve(config);
      }, config.requestOptions.retry.delay || 1);
    });
    config.headers = { ...config.headers, 'Content-Type': ContentTypeEnum.Json };
    return backoff.then((requestConfig) => instance.request(requestConfig));
  },
};

function createAxios(opt?: Partial<CreateAxiosOptions>) {
  return new VAxios(
    merge(
      <CreateAxiosOptions>{
        // https://developer.mozilla.org/en-US/docs/Web/HTTP/Authentication#authentication_schemes
        // 例如: authenticationScheme: 'Bearer'
        authenticationScheme: '',
        // 超时
        timeout: 10 * 1000,
        // 携带Cookie
        withCredentials: true,
        // 头信息
        headers: { 'Content-Type': ContentTypeEnum.Json },
        // 数据处理方式
        transform,
        // 配置项，下面的选项都可以在独立的接口请求中覆盖
        requestOptions: {
          // 接口地址
          apiUrl: host,
          // 是否自动添加接口前缀
          isJoinPrefix: true,
          // 接口前缀
          // 例如: https://www.baidu.com/api
          // urlPrefix: '/api'
          urlPrefix: apiPrefix,
          // 是否返回原生响应头 比如：需要获取响应头时使用该属性
          isReturnNativeResponse: false,
          // 需要对返回数据进行处理
          isTransformResponse: true,
          // post请求的时候添加参数到url
          joinParamsToUrl: false,
          // 格式化提交参数时间
          formatDate: true,
          // 是否加入时间戳
          joinTime: true,
          // 是否忽略请求取消令牌
          // 如果启用，则重复请求时不进行处理
          // 如果禁用，则重复请求时会取消当前请求
          ignoreCancelToken: true,
          // 是否携带token
          withToken: true,
          // 重试
          retry: {
            count: 3,
            delay: 1000,
          },
        },
      },
      opt || {},
    ),
  );
}
export const request = createAxios();
