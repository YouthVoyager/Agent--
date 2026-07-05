import { Fragment, h } from "../lib/react.js";

export function ObservabilityPanel({ observability, errorMessage }) {
  const enabled = observability?.enabled === true;
  const services = observability?.services ?? [];
  const dashboards = observability?.dashboards ?? [];
  const primaryDashboard = dashboards[0] ?? null;

  return h(
    "section",
    { className: "panel observability-panel" },
    h(
      "div",
      { className: "panel-heading" },
      h("h2", null, "观测系统"),
      h("span", { className: enabled ? "pill success" : "pill neutral" }, enabled ? "已启用" : "未启用"),
    ),
    errorMessage ? h("div", { className: "inline-error" }, errorMessage) : null,
    !enabled
      ? h("div", { className: "empty-state" }, "未配置观测栈")
      : h(
          Fragment,
          null,
          h(ServiceList, { services }),
          primaryDashboard ? h(DashboardPreview, { dashboard: primaryDashboard }) : null,
        ),
  );
}

function ServiceList({ services }) {
  if (!services.length) {
    return h("div", { className: "empty-state" }, "暂无观测服务");
  }

  return h(
    "div",
    { className: "observability-service-list" },
    ...services.map((service) =>
      h(
        "div",
        { className: "observability-service", key: `${service.name}-${service.kind}` },
        h(
          "div",
          null,
          h("strong", null, service.name),
          h("span", null, service.kind),
        ),
        h("span", { className: `status-badge ${statusTone(service.status)}` }, statusLabel(service.status)),
        service.url
          ? h(
              "a",
              {
                className: "text-link",
                href: service.url,
                rel: "noreferrer",
                target: "_blank",
              },
              "打开",
            )
          : h("span", { className: "muted-text" }, "无入口"),
      ),
    ),
  );
}

function DashboardPreview({ dashboard }) {
  return h(
    "div",
    { className: "dashboard-preview" },
    h(
      "div",
      { className: "observability-actions" },
      h("h3", null, dashboard.name),
      dashboard.url
        ? h(
            "a",
            {
              className: "secondary-link-button",
              href: dashboard.url,
              rel: "noreferrer",
              target: "_blank",
            },
            "打开 Grafana",
          )
        : null,
    ),
    dashboard.embedUrl
      ? h("iframe", {
          className: "observability-frame",
          loading: "lazy",
          src: dashboard.embedUrl,
          title: dashboard.name,
        })
      : h("div", { className: "empty-state" }, "未配置嵌入看板"),
  );
}

function statusLabel(status) {
  switch (status) {
    case "online":
      return "在线";
    case "degraded":
      return "异常";
    case "offline":
      return "离线";
    default:
      return "未知";
  }
}

function statusTone(status) {
  switch (status) {
    case "online":
      return "success";
    case "degraded":
      return "warning";
    case "offline":
      return "danger";
    default:
      return "neutral";
  }
}
