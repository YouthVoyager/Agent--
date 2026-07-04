import { Fragment, h } from "../lib/react.js";

export function MetricsPanel({ metrics, errorMessage }) {
  return h(
    "section",
    { className: "panel" },
    h("div", { className: "panel-heading" }, h("h2", null, "运行指标")),
    errorMessage ? h("div", { className: "inline-error" }, errorMessage) : null,
    metrics
      ? h(
          Fragment,
          null,
          h(
            "div",
            { className: "metric-list" },
            metricItem("累计请求", formatInteger(metrics.requests)),
            metricItem("模型降级", formatInteger(metrics.fallbacks)),
            metricItem("上游错误", formatInteger(metrics.upstreamErrors)),
            metricItem("Token 用量", formatInteger(metrics.tokenUsage)),
            metricItem("预算拒绝", formatInteger(metrics.budgetRejected)),
          ),
          h(SuccessRateList, { successRates: metrics.successRates }),
        )
      : h("div", { className: "empty-state" }, "暂无指标数据"),
  );
}

function SuccessRateList({ successRates }) {
  if (!successRates.length) {
    return null;
  }

  return h(
    "div",
    { className: "success-rate-list" },
    h("h3", null, "后端成功率"),
    ...successRates.map((item) =>
      h(
        "div",
        { className: "success-rate-row", key: item.backend },
        h("span", null, item.backend),
        h(
          "div",
          { className: "meter", "aria-label": `${item.backend} 成功率` },
          h("span", {
            style: { width: `${Math.max(0, Math.min(item.value, 1)) * 100}%` },
          }),
        ),
        h("strong", null, formatPercent(item.value)),
      ),
    ),
  );
}

function metricItem(label, value) {
  return h(
    "div",
    { className: "metric-item", key: label },
    h("span", null, label),
    h("strong", null, value),
  );
}

function formatInteger(value) {
  return new Intl.NumberFormat("zh-CN").format(value ?? 0);
}

function formatPercent(value) {
  return new Intl.NumberFormat("zh-CN", {
    maximumFractionDigits: 1,
    style: "percent",
  }).format(value ?? 0);
}
