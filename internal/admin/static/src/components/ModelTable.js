import { h } from "../lib/react.js";

export function ModelTable({ models, errorMessage }) {
  return h(
    "section",
    { className: "panel" },
    h(
      "div",
      { className: "panel-heading" },
      h("h2", null, "模型"),
      h("span", { className: "pill neutral" }, `${models.length} 个`),
    ),
    errorMessage ? h("div", { className: "inline-error" }, errorMessage) : null,
    models.length > 0
      ? h(
          "div",
          { className: "table-wrap" },
          h(
            "table",
            null,
            h(
              "thead",
              null,
              h("tr", null, h("th", null, "模型 ID"), h("th", null, "后端"), h("th", null, "创建时间")),
            ),
            h(
              "tbody",
              null,
              ...models.map((model) =>
                h(
                  "tr",
                  { key: model.id },
                  h("td", null, h("code", null, model.id)),
                  h("td", null, model.owner || "-"),
                  h("td", null, formatCreated(model.created)),
                ),
              ),
            ),
          ),
        )
      : h("div", { className: "empty-state" }, "暂无模型数据"),
  );
}

function formatCreated(created) {
  if (!created) {
    return "-";
  }

  return new Date(created * 1000).toLocaleString("zh-CN", {
    hour12: false,
  });
}
