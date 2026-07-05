import { createGatewayClient } from "../api/gatewayClient.js";
import { parseMetrics } from "../lib/metrics.js";
import { useCallback, useEffect, useState } from "../lib/react.js";

const refreshIntervalMilliseconds = 15000;

export function useGatewayData(apiKey) {
  const [data, setData] = useState(() => emptyData(true));

  const refresh = useCallback(async () => {
    setData((current) => ({
      ...current,
      loading: true,
    }));

    const client = createGatewayClient({ apiKey });
    const [health, readiness, models, metrics, observability] = await Promise.allSettled([
      client.getHealth(),
      client.getReady(),
      client.getModels(),
      client.getMetrics(),
      client.getObservability(),
    ]);

    const nextData = {
      health: settledValue(health),
      readiness: settledValue(readiness),
      models: normalizeModels(settledValue(models)?.data?.data),
      metrics: normalizeMetrics(settledValue(metrics)?.text),
      observability: normalizeObservability(settledValue(observability)?.data),
      errorMessage: firstErrorMessage([health, readiness]),
      modelErrorMessage: settledErrorMessage(models),
      metricsErrorMessage: settledErrorMessage(metrics),
      observabilityErrorMessage: settledErrorMessage(observability),
      loading: false,
      updatedAt: new Date(),
    };

    setData(nextData);
  }, [apiKey]);

  useEffect(() => {
    refresh();
    const intervalID = window.setInterval(refresh, refreshIntervalMilliseconds);
    return () => window.clearInterval(intervalID);
  }, [refresh]);

  return {
    data,
    refresh,
  };
}

function emptyData(loading) {
  return {
    health: null,
    readiness: null,
    models: [],
    metrics: null,
    observability: null,
    errorMessage: "",
    modelErrorMessage: "",
    metricsErrorMessage: "",
    observabilityErrorMessage: "",
    loading,
    updatedAt: null,
  };
}

function settledValue(result) {
  return result.status === "fulfilled" ? result.value : null;
}

function settledErrorMessage(result) {
  if (result.status === "fulfilled") {
    return "";
  }
  return result.reason?.message ?? "请求失败";
}

function firstErrorMessage(results) {
  const failed = results.find((result) => result.status === "rejected");
  return failed ? settledErrorMessage(failed) : "";
}

function normalizeModels(models) {
  if (!Array.isArray(models)) {
    return [];
  }

  return models.map((model) => ({
    id: model.id ?? "",
    owner: model.owned_by ?? "",
    created: model.created ?? 0,
  }));
}

function normalizeMetrics(metricsText) {
  if (!metricsText) {
    return null;
  }

  return parseMetrics(metricsText);
}

function normalizeObservability(observability) {
  if (!observability) {
    return null;
  }

  return {
    enabled: observability.enabled === true,
    services: Array.isArray(observability.services)
      ? observability.services.map((service) => ({
          name: service.name ?? "",
          kind: service.kind ?? "",
          url: service.url ?? "",
          status: service.status ?? "unknown",
          statusCode: service.status_code ?? 0,
          message: service.message ?? "",
        }))
      : [],
    dashboards: Array.isArray(observability.dashboards)
      ? observability.dashboards.map((dashboard) => ({
          name: dashboard.name ?? "",
          url: dashboard.url ?? "",
          embedUrl: dashboard.embed_url ?? "",
        }))
      : [],
  };
}
