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
    const [health, readiness, models, metrics] = await Promise.allSettled([
      client.getHealth(),
      client.getReady(),
      client.getModels(),
      client.getMetrics(),
    ]);

    const nextData = {
      health: settledValue(health),
      readiness: settledValue(readiness),
      models: normalizeModels(settledValue(models)?.data?.data),
      metrics: normalizeMetrics(settledValue(metrics)?.text),
      errorMessage: firstErrorMessage([health, readiness]),
      modelErrorMessage: settledErrorMessage(models),
      metricsErrorMessage: settledErrorMessage(metrics),
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
    errorMessage: "",
    modelErrorMessage: "",
    metricsErrorMessage: "",
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
