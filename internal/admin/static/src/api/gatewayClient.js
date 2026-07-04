export function createGatewayClient({ apiKey = "" } = {}) {
  return {
    getHealth: () => requestJSON("/healthz"),
    getReady: () => requestJSON("/readyz", { acceptedStatuses: [200, 503] }),
    getMetrics: () => requestText("/metrics"),
    getModels: () => requestJSON("/v1/models", { apiKey }),
  };
}

async function requestJSON(path, { acceptedStatuses = [200], apiKey = "" } = {}) {
  const response = await fetch(path, {
    headers: buildHeaders(apiKey),
  });
  const data = await parseJSON(response);
  if (!acceptedStatuses.includes(response.status)) {
    throw buildHTTPError(response, data);
  }

  return {
    data,
    status: response.status,
    traceID: response.headers.get("X-Trace-ID") ?? "",
  };
}

async function requestText(path) {
  const response = await fetch(path);
  const text = await response.text();
  if (!response.ok) {
    throw buildHTTPError(response, text);
  }

  return {
    text,
    status: response.status,
  };
}

function buildHeaders(apiKey) {
  const normalizedApiKey = apiKey.trim();
  if (normalizedApiKey === "") {
    return {};
  }

  const authorization = normalizedApiKey.toLowerCase().startsWith("bearer ")
    ? normalizedApiKey
    : `Bearer ${normalizedApiKey}`;

  return {
    Authorization: authorization,
  };
}

async function parseJSON(response) {
  const text = await response.text();
  if (text === "") {
    return null;
  }

  try {
    return JSON.parse(text);
  } catch (error) {
    throw new Error(`响应不是有效 JSON: ${error.message}`);
  }
}

function buildHTTPError(response, body) {
  const message = body?.error?.message || body?.status || response.statusText || "请求失败";
  const error = new Error(`${response.status} ${message}`);
  error.status = response.status;
  error.body = body;
  return error;
}
