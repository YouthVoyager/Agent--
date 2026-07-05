import { StatusCard } from "./StatusCard.js";
import { h } from "../lib/react.js";

export function StatusGrid({ health, readiness, metrics, modelCount, observability }) {
  const healthStatus = health?.data?.status ?? "unknown";
  const readyStatus = readiness?.data?.status ?? "unknown";
  const upStatus = metrics?.up === 1 ? "up" : "unknown";
  const observabilitySummary = summarizeObservability(observability);

  return h(
    "section",
    { className: "status-grid", "aria-label": "网关状态" },
    h(StatusCard, {
      label: "健康状态",
      value: healthStatus === "ok" ? "正常" : "未知",
      detail: health?.data?.service ?? "healthz",
      tone: healthStatus === "ok" ? "success" : "warning",
    }),
    h(StatusCard, {
      label: "就绪状态",
      value: readyStatus === "ready" ? "已就绪" : "未就绪",
      detail: readiness?.status ? `HTTP ${readiness.status}` : "readyz",
      tone: readyStatus === "ready" ? "success" : "warning",
    }),
    h(StatusCard, {
      label: "运行指标",
      value: upStatus === "up" ? "在线" : "未知",
      detail: metrics?.ready === 1 ? "ready=1" : "metrics",
      tone: upStatus === "up" ? "success" : "neutral",
    }),
    h(StatusCard, {
      label: "模型数量",
      value: `${modelCount}`,
      detail: "OpenAI 兼容模型",
      tone: modelCount > 0 ? "accent" : "neutral",
    }),
    h(StatusCard, {
      label: "观测系统",
      value: observabilitySummary.value,
      detail: observabilitySummary.detail,
      tone: observabilitySummary.tone,
    }),
  );
}

function summarizeObservability(observability) {
  if (!observability?.enabled) {
    return {
      value: "未启用",
      detail: "observability.stack",
      tone: "neutral",
    };
  }
  const services = observability.services ?? [];
  const online = services.filter((service) => service.status === "online").length;
  return {
    value: `${online}/${services.length}`,
    detail: "服务在线",
    tone: online === services.length && services.length > 0 ? "success" : "warning",
  };
}
