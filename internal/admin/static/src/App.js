import { ApiKeyPanel } from "./components/ApiKeyPanel.js";
import { ErrorBanner } from "./components/ErrorBanner.js";
import { MetricsPanel } from "./components/MetricsPanel.js";
import { ModelTable } from "./components/ModelTable.js";
import { ObservabilityPanel } from "./components/ObservabilityPanel.js";
import { StatusGrid } from "./components/StatusGrid.js";
import { useGatewayData } from "./hooks/useGatewayData.js";
import { h, useCallback, useState } from "./lib/react.js";

const apiKeyStorageKey = "agent-gateway.admin.api-key";

export function App() {
  const [apiKey, setApiKey] = useState(() => loadApiKey());
  const { data, refresh } = useGatewayData(apiKey);

  const handleApiKeyChange = useCallback((nextApiKey) => {
    const normalizedApiKey = nextApiKey.trim();
    setApiKey(normalizedApiKey);
    if (normalizedApiKey === "") {
      window.localStorage.removeItem(apiKeyStorageKey);
      return;
    }
    window.localStorage.setItem(apiKeyStorageKey, normalizedApiKey);
  }, []);

  return h(
    "div",
    { className: "app-shell" },
    h(
      "header",
      { className: "topbar" },
      h(
        "div",
        { className: "brand" },
        h("span", { className: "brand-mark", "aria-hidden": "true" }, "AG"),
        h(
          "div",
          null,
          h("p", { className: "eyebrow" }, "Telemetry Gateway"),
          h("h1", null, "Agent 网关管理"),
        ),
      ),
      h(
        "div",
        { className: "topbar-actions" },
        h("span", { className: "updated-at" }, formatUpdatedAt(data.updatedAt)),
        h(
          "button",
          {
            className: "primary-button",
            disabled: data.loading,
            type: "button",
            onClick: refresh,
          },
          data.loading ? "刷新中" : "刷新",
        ),
      ),
    ),
    h(
      "main",
      { className: "dashboard" },
      data.errorMessage
        ? h(ErrorBanner, { title: "服务状态读取失败", message: data.errorMessage })
        : null,
      h(StatusGrid, {
        health: data.health,
        readiness: data.readiness,
        metrics: data.metrics,
        modelCount: data.models.length,
        observability: data.observability,
      }),
      h(
        "div",
        { className: "content-grid" },
        h(
          "div",
          { className: "main-column" },
          h(ModelTable, {
            models: data.models,
            errorMessage: data.modelErrorMessage,
          }),
          h(MetricsPanel, {
            metrics: data.metrics,
            errorMessage: data.metricsErrorMessage,
          }),
          h(ObservabilityPanel, {
            observability: data.observability,
            errorMessage: data.observabilityErrorMessage,
          }),
        ),
        h(
          "aside",
          { className: "side-column" },
          h(ApiKeyPanel, {
            apiKey,
            onApiKeyChange: handleApiKeyChange,
          }),
          h(EndpointPanel),
        ),
      ),
    ),
  );
}

function EndpointPanel() {
  return h(
    "section",
    { className: "panel endpoint-panel" },
    h("div", { className: "panel-heading" }, h("h2", null, "接口")),
    h(
      "div",
      { className: "endpoint-list" },
      endpointItem("健康检查", "GET", "/healthz"),
      endpointItem("就绪状态", "GET", "/readyz"),
      endpointItem("模型列表", "GET", "/v1/models"),
      endpointItem("运行指标", "GET", "/metrics"),
      endpointItem("观测状态", "GET", "/admin/api/observability"),
    ),
  );
}

function endpointItem(label, method, url) {
  return h(
    "div",
    { className: "endpoint-item", key: url },
    h("span", { className: "method-chip" }, method),
    h("div", null, h("strong", null, label), h("code", null, url)),
  );
}

function loadApiKey() {
  return window.localStorage.getItem(apiKeyStorageKey) ?? "";
}

function formatUpdatedAt(updatedAt) {
  if (!updatedAt) {
    return "尚未刷新";
  }

  return `更新于 ${updatedAt.toLocaleTimeString("zh-CN", {
    hour12: false,
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  })}`;
}
